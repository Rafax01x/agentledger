package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"agentledger/internal/gitx"
	"agentledger/internal/store"
)

// Server serves the inspector API and static page over a store.
type Server struct {
	Root  string
	Port  int
	Store *store.Store
	Undo  UndoFunc
}

type sessionSummary struct {
	ID           string   `json:"id"`
	Agent        string   `json:"agent"`
	Command      string   `json:"command"`
	StartedAt    string   `json:"started_at"`
	DurationMs   int64    `json:"duration_ms"`
	ExitCode     *int     `json:"exit_code"`
	FilesChanged int      `json:"files_changed"`
	LinesAdded   *int     `json:"lines_added,omitempty"`
	LinesDeleted *int     `json:"lines_deleted,omitempty"`
	InputTokens  *int64   `json:"input_tokens,omitempty"`
	OutputTokens *int64   `json:"output_tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	UndoneAt     *string  `json:"undone_at,omitempty"`
	Status       string   `json:"status"`
}

func statusOf(s *store.Session) string {
	if s.UndoneAt != nil {
		return "undone"
	}
	if s.ExitCode != nil && *s.ExitCode != 0 {
		return "failed"
	}
	if s.EndedAt == nil {
		return "running"
	}
	return "ok"
}

func summarize(s *store.Session) sessionSummary {
	var undone *string
	if s.UndoneAt != nil {
		v := s.UndoneAt.UTC().Format(time.RFC3339)
		undone = &v
	}
	return sessionSummary{
		ID: s.ID, Agent: s.Agent, Command: s.Command,
		StartedAt:  s.StartedAt.UTC().Format(time.RFC3339),
		DurationMs: s.DurationMs, ExitCode: s.ExitCode,
		FilesChanged: s.FilesChanged, LinesAdded: s.LinesAdded,
		LinesDeleted: s.LinesDeleted, InputTokens: s.InputTokens,
		OutputTokens: s.OutputTokens, CostUSD: s.CostUSD,
		UndoneAt: undone, Status: statusOf(s),
	}
}

type actionJSON struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Diff    string `json:"diff"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

type eventJSON struct {
	Stream string `json:"stream"`
	Line   string `json:"line"`
}

type detailJSON struct {
	Session sessionSummary `json:"session"`
	Actions []actionJSON   `json:"actions"`
	Events  []eventJSON    `json:"events"`
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

// Handler returns the HTTP handler for the inspector. Sessions are
// listed newest first; detail and undo are addressed by session id.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFS.ReadFile("static/index.html")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/api/meta", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		sessions, err := s.Store.ListSessions(0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"root": s.Root, "port": s.Port, "sessions": len(sessions)})
	})
	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
			return
		}
		sessions, err := s.Store.ListSessions(0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out := make([]sessionSummary, 0, len(sessions))
		for _, sess := range sessions {
			out = append(out, summarize(sess))
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		id, action, _ := strings.Cut(rest, "/")
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		switch {
		case r.Method == http.MethodGet && action == "":
			sess, err := s.Store.GetSession(id)
			if err != nil {
				if errors.Is(err, store.ErrSessionNotFound) {
					writeError(w, http.StatusNotFound, err)
					return
				}
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			actions, err := s.Store.ListActions(id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			events, err := s.Store.ListEvents(id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			d := detailJSON{Session: summarize(sess)}
			for _, a := range actions {
				added, deleted := gitx.DiffStat(a.Diff)
				d.Actions = append(d.Actions, actionJSON{Path: a.Path, Type: a.Type, Diff: a.Diff, Added: added, Deleted: deleted})
			}
			for _, e := range events {
				d.Events = append(d.Events, eventJSON{Stream: e.Stream, Line: e.Line})
			}
			if d.Actions == nil {
				d.Actions = []actionJSON{}
			}
			if d.Events == nil {
				d.Events = []eventJSON{}
			}
			writeJSON(w, http.StatusOK, d)
		case r.Method == http.MethodPost && action == "undo":
			if s.Undo == nil {
				writeError(w, http.StatusServiceUnavailable, fmt.Errorf("undo is not available"))
				return
			}
			var body struct {
				Force bool     `json:"force"`
				Files []string `json:"files"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			n, err := s.Undo(id, body.Force, body.Files)
			if err != nil {
				if errors.Is(err, ErrDirty) {
					writeError(w, http.StatusConflict, err)
					return
				}
				if errors.Is(err, store.ErrSessionNotFound) {
					writeError(w, http.StatusNotFound, err)
					return
				}
				writeError(w, http.StatusBadRequest, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"files_restored": n})
		default:
			writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		}
	})
	return mux
}
