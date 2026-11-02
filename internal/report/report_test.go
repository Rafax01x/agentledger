package report_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agentledger/internal/report"
	"agentledger/internal/store"
)

func sampleSession() *store.Session {
	start := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	end := start.Add(1500 * time.Millisecond)
	code := 0
	return &store.Session{
		ID: "sess1", Agent: "claude", Command: "claude fix bug", Cwd: "/tmp/repo",
		StartedAt: start, EndedAt: &end, DurationMs: 1500, ExitCode: &code,
		StartRef: "refs/agentledger/sess1/start", EndRef: "refs/agentledger/sess1/end",
		FilesChanged: 1,
	}
}

func TestMarkdownContainsFieldsDiffAndOutput(t *testing.T) {
	sess := sampleSession()
	actions := []*store.Action{{ID: "a1", SessionID: "sess1", Type: "modified", Path: "x.txt", Diff: "--- a/x.txt\n+++ b/x.txt\n+new line"}}
	events := []*store.Event{
		{ID: "e1", SessionID: "sess1", Stream: "stdout", Line: "hello"},
		{ID: "e2", SessionID: "sess1", Stream: "stderr", Line: "oops"},
	}
	var buf bytes.Buffer
	if err := report.WriteMarkdown(&buf, sess, actions, events); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"# Session sess1", "claude", "claude fix bug",
		"x.txt (modified)", "new line",
		"[stdout] hello", "[stderr] oops",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("markdown missing %q:\n%s", want, out)
		}
	}
}

func TestJSONLDoesNotEscapeHTML(t *testing.T) {
	sess := sampleSession()
	sess.Command = "echo <tag> & more"
	actions := []*store.Action{{ID: "a1", SessionID: "sess1", Type: "modified", Path: "x.txt", Diff: "line <REDACTED:AWS_KEY>"}}
	var buf bytes.Buffer
	if err := report.WriteJSONL(&buf, sess, actions, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `\u003c`) {
		t.Fatalf("jsonl HTML-escaped angle brackets:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "<REDACTED:AWS_KEY>") {
		t.Fatalf("jsonl missing literal placeholder:\n%s", buf.String())
	}
}

func TestJSONLLineOrderAndShape(t *testing.T) {
	sess := sampleSession()
	actions := []*store.Action{{ID: "a1", SessionID: "sess1", Type: "added", Path: "y.txt", Diff: "diff-here"}}
	events := []*store.Event{{ID: "e1", SessionID: "sess1", Stream: "stdout", Line: "hi"}}
	var buf bytes.Buffer
	if err := report.WriteJSONL(&buf, sess, actions, events); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("jsonl lines = %d, want 3:\n%s", len(lines), buf.String())
	}
	var objs []map[string]any
	for _, line := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("line is not JSON: %s: %v", line, err)
		}
		objs = append(objs, obj)
	}
	if objs[0]["type"] != "session" || objs[0]["id"] != "sess1" {
		t.Fatalf("first line = %v", objs[0])
	}
	if objs[1]["type"] != "action" || objs[1]["path"] != "y.txt" {
		t.Fatalf("second line = %v", objs[1])
	}
	if objs[2]["type"] != "event" || objs[2]["line"] != "hi" {
		t.Fatalf("third line = %v", objs[2])
	}
}
