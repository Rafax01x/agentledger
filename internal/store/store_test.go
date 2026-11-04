package store_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"agentledger/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionRoundTrip(t *testing.T) {
	s := openTestStore(t)
	now := time.Now().Truncate(time.Millisecond)
	sess := &store.Session{
		ID: "sess1", Agent: "claude", Command: "claude",
		Cwd: "/tmp/repo", StartedAt: now, StartRef: "refs/agentledger/sess1/start",
	}
	if err := s.CreateSession(sess); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession("sess1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "claude" || got.Command != "claude" || got.Cwd != "/tmp/repo" {
		t.Fatalf("got %+v", got)
	}
	if !got.StartedAt.Equal(now.UTC()) {
		t.Fatalf("StartedAt = %v, want %v", got.StartedAt, now.UTC())
	}
	if got.EndedAt != nil || got.ExitCode != nil || got.UndoneAt != nil {
		t.Fatalf("unset fields should be nil: %+v", got)
	}
	if _, err := s.GetSession("missing"); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("GetSession missing err = %v, want ErrSessionNotFound", err)
	}
}

func TestFinishAndMarkUndone(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	if err := s.CreateSession(&store.Session{ID: "s1", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	end := now.Add(2 * time.Second)
	if err := s.FinishSession("s1", end, 2000, 3, "refs/agentledger/s1/end", 5); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.DurationMs != 2000 || got.ExitCode == nil || *got.ExitCode != 3 ||
		got.EndRef != "refs/agentledger/s1/end" || got.FilesChanged != 5 {
		t.Fatalf("after finish: %+v", got)
	}
	if err := s.FinishSession("nope", end, 0, 0, "", 0); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("FinishSession missing err = %v", err)
	}
	undone := end.Add(time.Minute)
	if err := s.MarkUndone("s1", undone); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetSession("s1")
	if err != nil {
		t.Fatal(err)
	}
	if got.UndoneAt == nil || !got.UndoneAt.Equal(undone.UTC()) {
		t.Fatalf("UndoneAt = %v, want %v", got.UndoneAt, undone.UTC())
	}
}

func TestListSessionsNewestFirstAndLimit(t *testing.T) {
	s := openTestStore(t)
	base := time.Now()
	for i, id := range []string{"old", "mid", "new"} {
		if err := s.CreateSession(&store.Session{ID: id, StartedAt: base.Add(time.Duration(i) * time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	all, err := s.ListSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != "new" || all[1].ID != "mid" || all[2].ID != "old" {
		t.Fatalf("ListSessions order wrong: %v", ids(all))
	}
	limited, err := s.ListSessions(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].ID != "new" {
		t.Fatalf("ListSessions(2) = %v", ids(limited))
	}
}

func ids(sessions []*store.Session) []string {
	out := make([]string, len(sessions))
	for i, s := range sessions {
		out[i] = s.ID
	}
	return out
}

func TestActionsAndEventsOrder(t *testing.T) {
	s := openTestStore(t)
	if err := s.CreateSession(&store.Session{ID: "s1", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for _, a := range []*store.Action{
		{ID: "a1", SessionID: "s1", Type: "modified", Path: "x.txt", Diff: "d1", CreatedAt: now},
		{ID: "a2", SessionID: "s1", Type: "added", Path: "y.txt", Diff: "d2", CreatedAt: now},
	} {
		if err := s.CreateAction(a); err != nil {
			t.Fatal(err)
		}
	}
	actions, err := s.ListActions("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0].Path != "x.txt" || actions[1].Path != "y.txt" {
		t.Fatalf("actions = %+v", actions)
	}
	for _, e := range []*store.Event{
		{ID: "e1", SessionID: "s1", Stream: "stdout", Line: "hello", CreatedAt: now},
		{ID: "e2", SessionID: "s1", Stream: "stderr", Line: "oops", CreatedAt: now},
	} {
		if err := s.CreateEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	events, err := s.ListEvents("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Stream != "stdout" || events[1].Line != "oops" {
		t.Fatalf("events = %+v", events)
	}
}

func TestPersistenceAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	s1, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.CreateSession(&store.Session{ID: "keep", Agent: "x", StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	s1.Close()
	s2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.GetSession("keep")
	if err != nil {
		t.Fatal(err)
	}
	if got.Agent != "x" {
		t.Fatalf("after reopen agent = %q", got.Agent)
	}
}

func TestDeleteSessionAndStoredBytes(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	if err := s.CreateSession(&store.Session{ID: "s1", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAction(&store.Action{ID: "a1", SessionID: "s1", Diff: "hello diff", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEvent(&store.Event{ID: "e1", SessionID: "s1", Line: "hello line", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	b, err := s.SessionStoredBytes("s1")
	if err != nil {
		t.Fatal(err)
	}
	if b != int64(len("hello diff")+len("hello line")) {
		t.Fatalf("SessionStoredBytes = %d", b)
	}
	if err := s.DeleteSession("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession("s1"); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("GetSession after delete err = %v", err)
	}
	if err := s.DeleteSession("s1"); !errors.Is(err, store.ErrSessionNotFound) {
		t.Fatalf("second DeleteSession err = %v", err)
	}
	if err := s.Vacuum(); err != nil {
		t.Fatal(err)
	}
}

func TestFinishSessionWithMetricsRoundTrip(t *testing.T) {
	s := openTestStore(t)
	now := time.Now()
	if err := s.CreateSession(&store.Session{ID: "m1", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	linesAdded, linesDeleted := 12, 3
	in, out := int64(3000), int64(400)
	cost := 0.042
	if err := s.FinishSessionWithMetrics("m1", now.Add(time.Second), 1000, 0, "refs/agentledger/m1/end", 2, &linesAdded, &linesDeleted, &in, &out, &cost); err != nil {
		t.Fatal(err)
	}
	sess, err := s.GetSession("m1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.LinesAdded == nil || *sess.LinesAdded != 12 || sess.LinesDeleted == nil || *sess.LinesDeleted != 3 {
		t.Fatalf("lines = %+v/%+v", sess.LinesAdded, sess.LinesDeleted)
	}
	if sess.InputTokens == nil || *sess.InputTokens != 3000 || sess.OutputTokens == nil || *sess.OutputTokens != 400 {
		t.Fatalf("tokens = %+v/%+v", sess.InputTokens, sess.OutputTokens)
	}
	if sess.CostUSD == nil || *sess.CostUSD != 0.042 {
		t.Fatalf("cost = %+v", sess.CostUSD)
	}
	// A session finished the old way reports nil telemetry, not zero.
	if err := s.CreateSession(&store.Session{ID: "m2", StartedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSession("m2", now, 500, 0, "refs/agentledger/m2/end", 0); err != nil {
		t.Fatal(err)
	}
	sess2, err := s.GetSession("m2")
	if err != nil {
		t.Fatal(err)
	}
	if sess2.LinesAdded != nil || sess2.InputTokens != nil || sess2.CostUSD != nil {
		t.Fatalf("old-style finish should leave telemetry nil: %+v", sess2)
	}
}

// TestOpenMigratesPreV02Database builds a database with the v0.1
// schema (no telemetry columns), inserts a session, then opens it
// through Store: the migration must add the columns and the old
// session must read back with nil telemetry.
func TestOpenMigratesPreV02Database(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/old.db"
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	oldSchema := `
CREATE TABLE sessions (
	id TEXT PRIMARY KEY, agent TEXT NOT NULL DEFAULT '', command TEXT NOT NULL DEFAULT '',
	cwd TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, ended_at TEXT, duration_ms INTEGER NOT NULL DEFAULT 0,
	exit_code INTEGER, start_ref TEXT NOT NULL DEFAULT '', end_ref TEXT NOT NULL DEFAULT '',
	files_changed INTEGER NOT NULL DEFAULT 0, undone_at TEXT);
CREATE TABLE actions (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, type TEXT NOT NULL DEFAULT '', path TEXT NOT NULL DEFAULT '', diff TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
CREATE TABLE events (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, stream TEXT NOT NULL DEFAULT '', line TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);`
	if _, err := raw.Exec(oldSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO sessions (id, started_at, files_changed) VALUES ('old1', '2026-01-01T00:00:00Z', 3)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sess, err := s.GetSession("old1")
	if err != nil {
		t.Fatal(err)
	}
	if sess.FilesChanged != 3 || sess.LinesAdded != nil || sess.InputTokens != nil || sess.CostUSD != nil {
		t.Fatalf("migrated session = %+v", sess)
	}
}
