package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// execCLI runs the root command in-process and returns its output.
// Global flag state is reset afterwards.
func execCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	rootCmd.SetArgs(nil)
	// Reset flag variables that commands may have changed.
	logLimit = 20
	showDiff = false
	showEvents = false
	exportFormat = ""
	undoFiles = nil
	undoDryRun = false
	undoForce = false
	return buf.String(), err
}

func runGitE2E(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func readE2E(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestUndoEndToEnd is the spec's M5 end-to-end test, driven through
// the real CLI commands in-process:
// init -> run fake agent (edits a.txt, adds new.txt) -> log -> show
// -> undo --dry-run (no change) -> undo (restored) -> run again,
// edit after the session, undo refuses, undo --force restores.
func TestUndoEndToEnd(t *testing.T) {
	root := t.TempDir()
	runGitE2E(t, root, "init", "-b", "main")
	runGitE2E(t, root, "config", "user.name", "test")
	runGitE2E(t, root, "config", "user.email", "test@localhost")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitE2E(t, root, "add", "a.txt")
	runGitE2E(t, root, "commit", "-m", "initial")
	// Read the fake agent BEFORE chdir: relative paths resolve
	// against the package directory only until t.Chdir runs.
	fakeSrc, err := os.ReadFile("../session/testdata/fake-agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	// init
	if _, err := execCLI(t, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// fake agent, copied in before the run (as in session tests).
	if err := os.WriteFile(filepath.Join(root, "fake-agent.sh"), fakeSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	// run
	if _, err := execCLI(t, "run", "--", "./fake-agent.sh"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := readE2E(t, root, "a.txt"); !strings.Contains(got, "edited by agent") {
		t.Fatalf("after run a.txt = %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); err != nil {
		t.Fatalf("after run new.txt missing: %v", err)
	}
	// log
	logOut, err := execCLI(t, "log")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if !strings.Contains(logOut, "fake-agent.sh") || !strings.Contains(logOut, "2 file(s)") {
		t.Fatalf("log output = %q", logOut)
	}
	// show
	showOut, err := execCLI(t, "show", "last")
	if err != nil {
		t.Fatalf("show: %v", err)
	}
	if !strings.Contains(showOut, "a.txt") || !strings.Contains(showOut, "new.txt") {
		t.Fatalf("show output = %q", showOut)
	}
	// undo --dry-run: must not change anything.
	dryOut, err := execCLI(t, "undo", "last", "--dry-run")
	if err != nil {
		t.Fatalf("undo --dry-run: %v", err)
	}
	if !strings.Contains(dryOut, "would restore") || !strings.Contains(dryOut, "no changes made") {
		t.Fatalf("dry-run output = %q", dryOut)
	}
	if got := readE2E(t, root, "a.txt"); !strings.Contains(got, "edited by agent") {
		t.Fatalf("dry-run changed a.txt: %q", got)
	}
	// undo --file for only new.txt: a.txt must stay edited.
	if _, err := execCLI(t, "undo", "last", "--file", "new.txt"); err != nil {
		t.Fatalf("undo --file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("undo --file new.txt should have deleted new.txt")
	}
	if got := readE2E(t, root, "a.txt"); !strings.Contains(got, "edited by agent") {
		t.Fatalf("undo --file should not touch a.txt: %q", got)
	}
	// undo the rest: a.txt restored, session marked undone.
	if _, err := execCLI(t, "undo", "last"); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if got := readE2E(t, root, "a.txt"); got != "one\n" {
		t.Fatalf("after undo a.txt = %q, want %q", got, "one\n")
	}
	logOut, err = execCLI(t, "log")
	if err != nil {
		t.Fatalf("log after undo: %v", err)
	}
	if !strings.Contains(logOut, "undone") {
		t.Fatalf("log after undo should mark undone: %q", logOut)
	}

	// Second session, then a post-session edit: undo must refuse.
	if _, err := execCLI(t, "run", "--", "./fake-agent.sh"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(root, "a.txt"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("user edit after session\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := execCLI(t, "undo", "last"); err == nil {
		t.Fatal("undo with post-session edits should refuse without --force")
	}
	if got := readE2E(t, root, "a.txt"); !strings.Contains(got, "user edit after session") {
		t.Fatalf("refused undo must not change a.txt: %q", got)
	}
	// --force overwrites the post-session edit.
	if _, err := execCLI(t, "undo", "last", "--force"); err != nil {
		t.Fatalf("undo --force: %v", err)
	}
	if got := readE2E(t, root, "a.txt"); got != "one\n" {
		t.Fatalf("after undo --force a.txt = %q, want %q", got, "one\n")
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("after undo --force new.txt should be gone")
	}
}
