package ui_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agentledger/internal/store"
	"agentledger/internal/ui"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/agentledger.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func seedSession(t *testing.T, st *store.Store) string {
	t.Helper()
	now := time.Now()
	if err := st.CreateSession(&store.Session{ID: "sess1", Agent: "claude", Command: "claude", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAction(&store.Action{ID: "a1", SessionID: "sess1", Type: "modified", Path: "a.txt", Diff: "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-one\n+two\n", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateEvent(&store.Event{ID: "e1", SessionID: "sess1", Stream: "stdout", Line: "hello", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	added, deleted := 1, 1
	if err := st.FinishSessionWithMetrics("sess1", now, 500, 0, "refs/agentledger/sess1/end", 1, &added, &deleted, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	return "sess1"
}

func get(t *testing.T, srv *ui.Server, path string) (int, []byte) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Code, body
}

func TestServerListDetailAndUndo(t *testing.T) {
	st := openStore(t)
	id := seedSession(t, st)
	undoCalled := false
	srv := &ui.Server{Root: t.TempDir(), Store: st, Undo: func(sessionID string, force bool, files []string) (int, error) {
		undoCalled = true
		if sessionID != id {
			t.Errorf("undo session = %q", sessionID)
		}
		if len(files) != 0 {
			t.Errorf("full undo should pass no files, got %v", files)
		}
		return 1, nil
	}}

	code, body := get(t, srv, "/api/sessions")
	if code != 200 {
		t.Fatalf("list code = %d, body = %s", code, body)
	}
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["id"] != id || list[0]["status"] != "ok" {
		t.Fatalf("list = %s", body)
	}

	code, body = get(t, srv, "/api/sessions/"+id)
	if code != 200 {
		t.Fatalf("detail code = %d", code)
	}
	var detail struct {
		Session struct {
			LinesAdded int `json:"lines_added"`
		} `json:"session"`
		Actions []struct {
			Added   int `json:"added"`
			Deleted int `json:"deleted"`
		} `json:"actions"`
		Events []struct {
			Line string `json:"line"`
		} `json:"events"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Session.LinesAdded != 1 || len(detail.Actions) != 1 || detail.Actions[0].Added != 1 || detail.Actions[0].Deleted != 1 {
		t.Fatalf("detail = %s", body)
	}
	if len(detail.Events) != 1 || detail.Events[0].Line != "hello" {
		t.Fatalf("events = %s", body)
	}

	code, body = get(t, srv, "/api/sessions/nope")
	if code != 404 {
		t.Fatalf("unknown session code = %d", code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+id+"/undo", strings.NewReader(`{"force":false}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || !undoCalled {
		t.Fatalf("undo code = %d, called = %v", rec.Code, undoCalled)
	}
	if !strings.Contains(rec.Body.String(), `"files_restored":1`) {
		t.Fatalf("undo body = %s", rec.Body.String())
	}

	code, body = get(t, srv, "/")
	if code != 200 || !strings.Contains(string(body), "AgentLedger // Dev Inspector") {
		t.Fatalf("index code = %d", code)
	}
}

func TestServerUndoDirtyMapsTo409(t *testing.T) {
	st := openStore(t)
	seedSession(t, st)
	srv := &ui.Server{Store: st, Undo: func(string, bool, []string) (int, error) {
		return 0, ui.ErrDirty
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/sess1/undo", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 409 {
		t.Fatalf("dirty undo code = %d, want 409", rec.Code)
	}
}

func TestServerMetaAndSelectiveUndo(t *testing.T) {
	st := openStore(t)
	id := seedSession(t, st)
	var gotFiles []string
	srv := &ui.Server{Root: "/repo/root", Port: 4848, Store: st, Undo: func(sessionID string, force bool, files []string) (int, error) {
		gotFiles = files
		return len(files), nil
	}}
	code, body := get(t, srv, "/api/meta")
	if code != 200 {
		t.Fatalf("meta code = %d", code)
	}
	var meta struct {
		Root     string `json:"root"`
		Port     int    `json:"port"`
		Sessions int    `json:"sessions"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Root != "/repo/root" || meta.Port != 4848 || meta.Sessions != 1 {
		t.Fatalf("meta = %+v", meta)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+id+"/undo", strings.NewReader(`{"force":false,"files":["a.txt"]}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("selective undo code = %d", rec.Code)
	}
	if len(gotFiles) != 1 || gotFiles[0] != "a.txt" {
		t.Fatalf("selective undo files = %v", gotFiles)
	}
	if !strings.Contains(rec.Body.String(), `"files_restored":1`) {
		t.Fatalf("selective undo body = %s", rec.Body.String())
	}
}
