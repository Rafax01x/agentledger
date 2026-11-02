package report

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"agentledger/internal/store"
)

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatNullableTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}

func formatExitCode(code *int) string {
	if code == nil {
		return ""
	}
	return fmt.Sprintf("%d", *code)
}

// WriteMarkdown writes a human-readable report for one session:
// header fields, changed files with diffs, then captured output.
func WriteMarkdown(w io.Writer, sess *store.Session, actions []*store.Action, events []*store.Event) error {
	if _, err := fmt.Fprintf(w, "# Session %s\n\n", sess.ID); err != nil {
		return err
	}
	fields := []struct{ k, v string }{
		{"Agent", sess.Agent},
		{"Command", "`" + sess.Command + "`"},
		{"Cwd", sess.Cwd},
		{"Started", formatTime(sess.StartedAt)},
		{"Ended", formatNullableTime(sess.EndedAt)},
		{"Duration", fmt.Sprintf("%d ms", sess.DurationMs)},
		{"Exit code", formatExitCode(sess.ExitCode)},
		{"Start ref", sess.StartRef},
		{"End ref", sess.EndRef},
		{"Files changed", fmt.Sprintf("%d", sess.FilesChanged)},
		{"Undone at", formatNullableTime(sess.UndoneAt)},
	}
	for _, f := range fields {
		if _, err := fmt.Fprintf(w, "- %s: %s\n", f.k, f.v); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "\n## Files changed"); err != nil {
		return err
	}
	if len(actions) == 0 {
		if _, err := fmt.Fprintln(w, "\n(no files changed)"); err != nil {
			return err
		}
	}
	for _, a := range actions {
		if _, err := fmt.Fprintf(w, "\n### %s (%s)\n", a.Path, a.Type); err != nil {
			return err
		}
		if a.Diff != "" {
			if _, err := fmt.Fprintf(w, "\n```diff\n%s\n```\n", a.Diff); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(w, "\n## Output"); err != nil {
		return err
	}
	if len(events) == 0 {
		if _, err := fmt.Fprintln(w, "\n(no output captured)"); err != nil {
			return err
		}
	}
	for _, e := range events {
		if _, err := fmt.Fprintf(w, "- [%s] %s\n", e.Stream, e.Line); err != nil {
			return err
		}
	}
	return nil
}

// JSON shapes for jsonl export. One JSON object per line, in order:
// session, then actions, then events.
type sessionJSON struct {
	Type         string  `json:"type"`
	ID           string  `json:"id"`
	Agent        string  `json:"agent"`
	Command      string  `json:"command"`
	Cwd          string  `json:"cwd"`
	StartedAt    string  `json:"started_at"`
	EndedAt      *string `json:"ended_at"`
	DurationMs   int64   `json:"duration_ms"`
	ExitCode     *int    `json:"exit_code"`
	StartRef     string  `json:"start_ref"`
	EndRef       string  `json:"end_ref"`
	FilesChanged int     `json:"files_changed"`
	UndoneAt     *string `json:"undone_at"`
}

type actionJSON struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Action    string `json:"action_type"`
	Path      string `json:"path"`
	Diff      string `json:"diff"`
}

type eventJSON struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Stream    string `json:"stream"`
	Line      string `json:"line"`
}

func nullableString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t)
	return &s
}

func writeJSONLine(w io.Writer, v any) error {
	// An Encoder with HTML escaping disabled: the default json.Marshal
	// turns <, >, & into \u003c etc, which would make redaction
	// placeholders like <REDACTED:AWS_KEY> unreadable in the export
	// (they still parse back correctly, but nobody greps \u003c).
	// Encode writes its own trailing newline.
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// WriteJSONL writes one JSON object per line: the session, then its
// actions in order, then its events in order.
func WriteJSONL(w io.Writer, sess *store.Session, actions []*store.Action, events []*store.Event) error {
	if err := writeJSONLine(w, sessionJSON{
		Type: "session", ID: sess.ID, Agent: sess.Agent, Command: sess.Command,
		Cwd: sess.Cwd, StartedAt: formatTime(sess.StartedAt), EndedAt: nullableString(sess.EndedAt),
		DurationMs: sess.DurationMs, ExitCode: sess.ExitCode, StartRef: sess.StartRef,
		EndRef: sess.EndRef, FilesChanged: sess.FilesChanged, UndoneAt: nullableString(sess.UndoneAt),
	}); err != nil {
		return err
	}
	for _, a := range actions {
		if err := writeJSONLine(w, actionJSON{
			Type: "action", ID: a.ID, SessionID: a.SessionID,
			Action: a.Type, Path: a.Path, Diff: a.Diff,
		}); err != nil {
			return err
		}
	}
	for _, e := range events {
		if err := writeJSONLine(w, eventJSON{
			Type: "event", ID: e.ID, SessionID: e.SessionID,
			Stream: e.Stream, Line: e.Line,
		}); err != nil {
			return err
		}
	}
	return nil
}
