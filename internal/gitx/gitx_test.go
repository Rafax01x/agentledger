package gitx_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agentledger/internal/gitx"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// setupRepo creates a git repo with one committed file and returns its root.
func setupRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-b", "main")
	runGit(t, root, "config", "user.name", "test")
	runGit(t, root, "config", "user.email", "test@localhost")
	writeFile(t, root, "a.txt", "one\n")
	runGit(t, root, "add", "a.txt")
	runGit(t, root, "commit", "-m", "initial")
	return root
}

func TestFindRootAndHead(t *testing.T) {
	root := setupRepo(t)
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := gitx.FindRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if found != root {
		t.Fatalf("FindRoot = %q, want %q", found, root)
	}
	head, err := gitx.Head(root)
	if err != nil {
		t.Fatal(err)
	}
	if head != runGit(t, root, "rev-parse", "HEAD") {
		t.Fatalf("Head = %q, want HEAD", head)
	}
	if _, err := gitx.FindRoot(t.TempDir()); err == nil {
		t.Fatal("FindRoot outside a repo should fail")
	}
}

func TestSnapshotDoesNotTouchBranchAndRestore(t *testing.T) {
	root := setupRepo(t)
	headBefore := runGit(t, root, "rev-parse", "HEAD")

	// Change a tracked file and add an untracked one, then snapshot.
	writeFile(t, root, "a.txt", "two\n")
	writeFile(t, root, "b.txt", "new\n")
	commit, err := gitx.Snapshot(root, "testid123", "start")
	if err != nil {
		t.Fatal(err)
	}
	if commit == "" {
		t.Fatal("Snapshot returned empty commit")
	}
	// Branch, index and HEAD must be untouched.
	if got := runGit(t, root, "rev-parse", "HEAD"); got != headBefore {
		t.Fatalf("HEAD moved: %q -> %q", headBefore, got)
	}
	if got := runGit(t, root, "rev-parse", "--verify", "refs/agentledger/testid123/start"); got != commit {
		t.Fatalf("snapshot ref = %q, want %q", got, commit)
	}
	// Snapshot must contain both the edit and the new file.
	if got := runGit(t, root, "show", commit+":a.txt"); got != "two" {
		t.Fatalf("snapshot a.txt = %q, want %q", got, "two")
	}
	if got := runGit(t, root, "show", commit+":b.txt"); got != "new" {
		t.Fatalf("snapshot b.txt = %q, want %q", got, "new")
	}
	// No temp index left behind.
	entries, err := os.ReadDir(filepath.Join(root, ".agentledger", "tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp index files left behind: %v", entries)
	}

	// ChangedFiles + Diff between HEAD and the snapshot.
	files, err := gitx.ChangedFiles(root, "HEAD", commit)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0] != "a.txt" || files[1] != "b.txt" {
		t.Fatalf("ChangedFiles = %v, want [a.txt b.txt]", files)
	}
	diff, err := gitx.Diff(root, "HEAD", commit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "two") {
		t.Fatalf("Diff missing edited content:\n%s", diff)
	}

	// Restore a.txt from the snapshot, then change it again and restore back.
	writeFile(t, root, "a.txt", "three\n")
	if err := gitx.Restore(root, commit, []string{"a.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, root, "a.txt"); got != "two\n" {
		t.Fatalf("after restore a.txt = %q, want %q", got, "two\n")
	}
	if err := gitx.Restore(root, commit, nil); err == nil {
		t.Fatal("Restore with no paths should fail")
	}
}

func TestSnapshotExcludesAgentledgerDir(t *testing.T) {
	root := setupRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".agentledger"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".agentledger/db.txt", "tool data\n")
	commit, err := gitx.Snapshot(root, "excl1", "start")
	if err != nil {
		t.Fatal(err)
	}
	// The snapshot tree must not contain the tool's own directory.
	tree := runGit(t, root, "ls-tree", "-r", "--name-only", commit)
	if strings.Contains(tree, ".agentledger") {
		t.Fatalf("snapshot contains .agentledger:\n%s", tree)
	}
}
