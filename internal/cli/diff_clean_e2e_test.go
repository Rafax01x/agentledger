package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupDiffCleanRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGitE2E(t, root, "init", "-b", "main")
	runGitE2E(t, root, "config", "user.name", "test")
	runGitE2E(t, root, "config", "user.email", "test@localhost")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitE2E(t, root, "add", "a.txt")
	runGitE2E(t, root, "commit", "-m", "initial")
	fakeSrc, err := os.ReadFile("../session/testdata/fake-agent.sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if _, err := execCLI(t, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "fake-agent.sh"), fakeSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := execCLI(t, "run", "--", "./fake-agent.sh"); err != nil {
		t.Fatalf("run: %v", err)
	}
	return root
}

func TestDiffAndClean(t *testing.T) {
	setupDiffCleanRepo(t)

	diffOut, err := execCLI(t, "diff", "last")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !strings.Contains(diffOut, "edited by agent") || !strings.Contains(diffOut, "a.txt") {
		t.Fatalf("diff output = %q", diffOut)
	}
	statOut, err := execCLI(t, "diff", "last", "--stat")
	if err != nil {
		t.Fatalf("diff --stat: %v", err)
	}
	if !strings.Contains(statOut, "a.txt +1") || !strings.Contains(statOut, "new.txt +1") {
		t.Fatalf("diff --stat output = %q", statOut)
	}

	// Dry run lists the session but deletes nothing.
	dryOut, err := execCLI(t, "clean", "--older-than", "0d", "--keep-last", "0", "--dry-run")
	if err != nil {
		t.Fatalf("clean --dry-run: %v", err)
	}
	if !strings.Contains(dryOut, "would delete") || !strings.Contains(dryOut, "no changes made") {
		t.Fatalf("clean dry-run output = %q", dryOut)
	}
	if logOut, err := execCLI(t, "log"); err != nil || strings.Contains(logOut, "No sessions") {
		t.Fatalf("dry run must not delete: log=%q err=%v", logOut, err)
	}
	// Real clean removes the session and its refs.
	cleanOut, err := execCLI(t, "clean", "--older-than", "0d", "--keep-last", "0")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if !strings.Contains(cleanOut, "Deleted 1 session") || !strings.Contains(cleanOut, "Reclaimed") {
		t.Fatalf("clean output = %q", cleanOut)
	}
	logOut, err := execCLI(t, "log")
	if err != nil {
		t.Fatalf("log after clean: %v", err)
	}
	if !strings.Contains(logOut, "No sessions") {
		t.Fatalf("log after clean = %q", logOut)
	}
	if _, err := execCLI(t, "diff", "last"); err == nil {
		t.Fatal("diff after clean should fail: session is gone")
	}
}

func TestCleanKeepsNewest(t *testing.T) {
	setupDiffCleanRepo(t)
	if _, err := execCLI(t, "run", "--", "./fake-agent.sh"); err != nil {
		t.Fatalf("second run: %v", err)
	}
	out, err := execCLI(t, "clean", "--older-than", "0d", "--keep-last", "1")
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if !strings.Contains(out, "Deleted 1 session") {
		t.Fatalf("clean should delete exactly the older session: %q", out)
	}
	logOut, err := execCLI(t, "log")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if strings.Contains(logOut, "No sessions") {
		t.Fatalf("newest session must survive clean: %q", logOut)
	}
}

func TestCleanRejectsBadAge(t *testing.T) {
	setupDiffCleanRepo(t)
	if _, err := execCLI(t, "clean", "--older-than", "soon"); err == nil {
		t.Fatal("clean with a bad age should fail")
	}
}
