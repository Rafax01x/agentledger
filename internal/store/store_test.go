package store_test

import (
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
