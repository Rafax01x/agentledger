package session_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agentledger/internal/session"
	"agentledger/internal/store"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func setupRepo(t *testing.T) (root string, dbPath string) {
	t.Helper()
	root = t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "test")
	runGit(t, root, "config", "user.email", "test@localhost")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "a.txt")
	runGit(t, root, "commit", "-m", "initial")
	// Store lives outside the repo working tree here so the test
	// store file itself is not part of the snapshot.
	dbPath = filepath.Join(t.TempDir(), "agentledger.db")
	return root, dbPath
}

func copyFakeAgent(t *testing.T, root string) string {
	t.Helper()
	src, err := os.ReadFile("testdata/fake-agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(root, "fake-agent.sh")
	if err := os.WriteFile(dst, src, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestRunCapturesOutputAndChanges(t *testing.T) {
	root, dbPath := setupRepo(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := copyFakeAgent(t, root)
	headBefore := func() string {
		cmd := exec.Command("git", "-C", root, "rev-parse", "HEAD")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}()

	var stdout, stderr bytes.Buffer
	res, err := session.Run(session.Config{
		Root: root, Cwd: root, Agent: "fake-agent",
		Command: []string{fake}, Env: os.Environ(),
		Stdout: &stdout, Stderr: &stderr, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", res.ExitCode)
	}
	// Live output must have been streamed to the writers.
	if !strings.Contains(stdout.String(), "agent stdout line") {
		t.Fatalf("live stdout = %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "agent stderr line") {
		t.Fatalf("live stderr = %q", stderr.String())
	}
	// HEAD and branch untouched.
	cmd := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != headBefore {
		t.Fatal("HEAD moved during run")
	}
	// Stored session fields.
	sess := res.Session
	if sess.ExitCode == nil || *sess.ExitCode != 0 {
		t.Fatalf("stored ExitCode = %+v", sess.ExitCode)
	}
	if sess.StartRef == "" || sess.EndRef == "" {
		t.Fatalf("refs missing: %+v", sess)
	}
	// The fake agent script was copied in before the run, so it is
	// already in the start snapshot; the recorded changes are only
	// a.txt (modified) + new.txt (added) = 2.
	if sess.FilesChanged != 2 {
		t.Fatalf("FilesChanged = %d, want 2", sess.FilesChanged)
	}
	actions, err := st.ListActions(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, a := range actions {
		paths[a.Path] = a.Type
	}
	if paths["a.txt"] != "modified" || paths["new.txt"] != "added" {
		t.Fatalf("actions = %v", paths)
	}
	// Diff for a.txt must contain the agent's edit.
	for _, a := range actions {
		if a.Path == "a.txt" && !strings.Contains(a.Diff, "edited by agent") {
			t.Fatalf("a.txt diff missing edit:\n%s", a.Diff)
		}
	}
	events, err := st.ListEvents(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	var gotStdout, gotStderr []string
	for _, e := range events {
		if e.Stream == "stdout" {
			gotStdout = append(gotStdout, e.Line)
		} else {
			gotStderr = append(gotStderr, e.Line)
		}
	}
	if len(gotStdout) != 1 || gotStdout[0] != "agent stdout line" {
		t.Fatalf("stdout events = %v", gotStdout)
	}
	if len(gotStderr) != 1 || gotStderr[0] != "agent stderr line" {
		t.Fatalf("stderr events = %v", gotStderr)
	}
	// Working tree really was changed by the agent.
	b, err := os.ReadFile(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "edited by agent") {
		t.Fatalf("a.txt = %q", b)
	}
}

func TestRunPropagatesExitCode(t *testing.T) {
	root, dbPath := setupRepo(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := copyFakeAgent(t, root)
	env := append(os.Environ(), "FAKE_EXIT=3")
	var stdout, stderr bytes.Buffer
	res, err := session.Run(session.Config{
		Root: root, Cwd: root, Agent: "fake-agent",
		Command: []string{fake}, Env: env,
		Stdout: &stdout, Stderr: &stderr, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", res.ExitCode)
	}
	if res.Session.ExitCode == nil || *res.Session.ExitCode != 3 {
		t.Fatalf("stored ExitCode = %+v", res.Session.ExitCode)
	}
}

func TestRunRedactsSecretsBeforePersist(t *testing.T) {
	root, dbPath := setupRepo(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	awsKey := "AKIAIOSFODNN7EXAMPLE"
	ghToken := "ghp_abcdefghijklmnopqrstuvwxyz0123456789"
	var stdout, stderr bytes.Buffer
	// Output line carries an AWS key; a created file carries a GitHub token.
	res, err := session.Run(session.Config{
		Root: root, Cwd: root, Agent: "sh",
		Command: []string{"sh", "-c", "echo key=" + awsKey + "; echo token=" + ghToken + " > token.txt"},
		Env:     os.Environ(), Stdout: &stdout, Stderr: &stderr, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Live output is NOT redacted: the user sees what ran.
	if !strings.Contains(stdout.String(), awsKey) {
		t.Fatalf("live stdout should show the raw key, got %q", stdout.String())
	}
	// Stored output IS redacted.
	events, err := st.ListEvents(res.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if strings.Contains(events[0].Line, awsKey) || !strings.Contains(events[0].Line, "<REDACTED:AWS_KEY>") {
		t.Fatalf("stored event = %q", events[0].Line)
	}
	// Stored diff IS redacted.
	actions, err := st.ListActions(res.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range actions {
		if a.Path == "token.txt" {
			found = true
			if strings.Contains(a.Diff, ghToken) || !strings.Contains(a.Diff, "<REDACTED:GITHUB_TOKEN>") {
				t.Fatalf("stored diff = %q", a.Diff)
			}
		}
	}
	if !found {
		t.Fatalf("token.txt action missing: %+v", actions)
	}
}

func TestRunMissingCommandRecords127(t *testing.T) {
	root, dbPath := setupRepo(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var stdout, stderr bytes.Buffer
	res, err := session.Run(session.Config{
		Root: root, Cwd: root, Agent: "missing",
		Command: []string{"definitely-not-a-real-binary-xyz"}, Env: os.Environ(),
		Stdout: &stdout, Stderr: &stderr, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 127 {
		t.Fatalf("ExitCode = %d, want 127", res.ExitCode)
	}
	if !strings.Contains(stderr.String(), "executable file not found") {
		t.Fatalf("stderr should explain the missing binary, got %q", stderr.String())
	}
}

func TestRunAppliesConfigIgnoreAndEnvRedaction(t *testing.T) {
	root, dbPath := setupRepo(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	secret := "configenvsecret777"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("API_KEY="+secret+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agentledger.yaml"), []byte("ignore_paths:\n  - \"ignored.txt\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	res, err := session.Run(session.Config{
		Root: root, Cwd: root, Agent: "sh",
		Command: []string{"sh", "-c", "echo leaked=" + secret + "; echo kept > kept.txt; echo ignored > ignored.txt"},
		Env:     os.Environ(), Stdout: &stdout, Stderr: &stderr, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	actions, err := st.ListActions(res.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range actions {
		if a.Path == "ignored.txt" {
			t.Fatalf("ignored file must not be recorded: %+v", actions)
		}
	}
	found := false
	for _, a := range actions {
		if a.Path == "kept.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("kept.txt action missing: %+v", actions)
	}
	events, err := st.ListEvents(res.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if strings.Contains(e.Line, secret) {
			t.Fatalf("env secret leaked in stored event: %q", e.Line)
		}
	}
}

func TestRunRecordsTelemetry(t *testing.T) {
	root, dbPath := setupRepo(t)
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var stdout, stderr bytes.Buffer
	res, err := session.Run(session.Config{
		Root: root, Cwd: root, Agent: "sh",
		Command: []string{"sh", "-c", "echo '{\"input_tokens\": 100, \"output_tokens\": 20}'; echo 'total cost: $0.011'; echo one >> a.txt; echo two >> a.txt"},
		Env:     os.Environ(), Stdout: &stdout, Stderr: &stderr, Store: st,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess := res.Session
	if sess.LinesAdded == nil || *sess.LinesAdded != 2 || sess.LinesDeleted == nil || *sess.LinesDeleted != 0 {
		t.Fatalf("lines = %+v/%+v, want +2 -0", sess.LinesAdded, sess.LinesDeleted)
	}
	if sess.InputTokens == nil || *sess.InputTokens != 100 || sess.OutputTokens == nil || *sess.OutputTokens != 20 {
		t.Fatalf("tokens = %+v/%+v", sess.InputTokens, sess.OutputTokens)
	}
	if sess.CostUSD == nil || *sess.CostUSD != 0.011 {
		t.Fatalf("cost = %+v, want 0.011", sess.CostUSD)
	}
}
