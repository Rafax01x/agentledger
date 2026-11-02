package gitx

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oklog/ulid/v2"
)

// run executes `git -C root args...` with the given extra environment
// and returns trimmed stdout. On failure it returns an actionable error
// naming the git subcommand, its stderr, and what to check.
func run(root string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %s: %w (check that %s is a git repository with a HEAD commit and that git is installed)",
			strings.Join(args, " "), strings.TrimSpace(stderr.String()), err, root)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// runRaw is run without trimming stdout, for commands whose exact
// bytes matter (for example `git show <ref>:<path>` content).
func runRaw(root string, extraEnv []string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %s: %w", strings.Join(args, " "), strings.TrimSpace(stderr.String()), err)
	}
	return stdout.String(), nil
}

// FindRoot returns the top-level directory of the git repository
// containing dir.
func FindRoot(dir string) (string, error) {
	out, err := run(dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not a git repository (no git root found from %s): %w", dir, err)
	}
	return out, nil
}

// Head returns the current HEAD commit hash of the repository at root.
func Head(root string) (string, error) {
	return run(root, nil, "rev-parse", "HEAD")
}

// Status returns `git status --porcelain` output for root: empty means
// a clean working tree, one line per changed file otherwise.
func Status(root string) (string, error) {
	return run(root, nil, "status", "--porcelain")
}

// contentMatchesRef reports whether a working-tree file's bytes
// match its content in ref (including both being absent).
func contentMatchesRef(root, ref, path string) (bool, error) {
	snapshot, snapErr := runRaw(root, nil, "show", ref+":"+path)
	working, workErr := os.ReadFile(filepath.Join(root, path))
	snapExists := snapErr == nil
	workExists := workErr == nil
	if workErr != nil && !os.IsNotExist(workErr) {
		return false, fmt.Errorf("read working file %s: %w", path, workErr)
	}
	if snapExists != workExists {
		return false, nil
	}
	if !snapExists {
		return true, nil
	}
	return snapshot == string(working), nil
}

// ListFiles lists all file paths in a snapshot commit's tree.
func ListFiles(root, ref string) ([]string, error) {
	out, err := run(root, nil, "ls-tree", "-r", "--name-only", ref)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ChangedFromRef lists target paths whose working-tree content
// differs from ref (for example a session's end snapshot), by
// comparing file bytes directly. This deliberately does NOT use
// `git diff <ref>`: that command routes through the user's normal
// index, so a file that is in the snapshot but untracked in the
// normal index (a file the session added) is falsely reported as
// changed. .agentledger/ paths are ignored: tool data, never part
// of a snapshot.
func ChangedFromRef(root, ref string, paths []string) ([]string, error) {
	if _, err := RefCommit(root, ref); err != nil {
		return nil, err
	}
	var changed []string
	for _, path := range paths {
		if path == ".agentledger" || strings.HasPrefix(path, ".agentledger/") {
			continue
		}
		matches, err := contentMatchesRef(root, ref, path)
		if err != nil {
			return nil, err
		}
		if !matches {
			changed = append(changed, path)
		}
	}
	return changed, nil
}

// ChangedSinceSession lists target paths whose working-tree content
// matches NEITHER the start nor the end snapshot: files the user
// changed again after the session ended. A file that already
// matches the start snapshot is already undone (for example after
// a partial --file undo) and does not block a later undo.
func ChangedSinceSession(root, startRef, endRef string, paths []string) ([]string, error) {
	if _, err := RefCommit(root, startRef); err != nil {
		return nil, err
	}
	if _, err := RefCommit(root, endRef); err != nil {
		return nil, err
	}
	var changed []string
	for _, path := range paths {
		if path == ".agentledger" || strings.HasPrefix(path, ".agentledger/") {
			continue
		}
		matchesEnd, err := contentMatchesRef(root, endRef, path)
		if err != nil {
			return nil, err
		}
		if matchesEnd {
			continue
		}
		matchesStart, err := contentMatchesRef(root, startRef, path)
		if err != nil {
			return nil, err
		}
		if !matchesStart {
			changed = append(changed, path)
		}
	}
	return changed, nil
}

// Snapshot records the current working tree (tracked and untracked
// files) as a commit on refs/agentledger/<id>/<name>, without touching
// the user's branch, index, stash, or commits.
//
// It follows the spec algorithm: a temporary index is built with
// read-tree HEAD + add -A + write-tree, a commit is created with
// commit-tree, and the ref is updated. The temporary index is deleted
// afterwards, including on error.
//
// The tool's own .agentledger/ directory is excluded from the snapshot
// (via a pathspec), otherwise the snapshot would recursively include
// agentledger's own database and temp files.
func Snapshot(root, id, name string) (string, error) {
	head, err := Head(root)
	if err != nil {
		return "", err
	}
	tmpDir := filepath.Join(root, ".agentledger", "tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", fmt.Errorf("create snapshot temp dir %s: %w", tmpDir, err)
	}
	tmpIndex := filepath.Join(tmpDir, "index."+ulid.Make().String())
	defer os.Remove(tmpIndex)
	env := []string{"GIT_INDEX_FILE=" + tmpIndex}
	if _, err := run(root, env, "read-tree", "HEAD"); err != nil {
		return "", err
	}
	if _, err := run(root, env, "add", "-A", "--", ":!.agentledger"); err != nil {
		return "", err
	}
	tree, err := run(root, env, "write-tree")
	if err != nil {
		return "", err
	}
	commit, err := run(root, nil, "commit-tree", tree, "-p", head, "-m", "agentledger snapshot "+id)
	if err != nil {
		return "", err
	}
	ref := "refs/agentledger/" + id + "/" + name
	if _, err := run(root, nil, "update-ref", ref, commit); err != nil {
		return "", err
	}
	return commit, nil
}

// RefCommit resolves a ref (for example refs/agentledger/<id>/start)
// to its commit hash.
func RefCommit(root, ref string) (string, error) {
	return run(root, nil, "rev-parse", "--verify", ref)
}

// ChangedFiles lists files that differ between two refs.
func ChangedFiles(root, fromRef, toRef string) ([]string, error) {
	out, err := run(root, nil, "diff", "--name-only", fromRef, toRef)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// Diff returns the unified diff between two refs.
func Diff(root, fromRef, toRef string) (string, error) {
	return run(root, nil, "diff", "--no-color", fromRef, toRef)
}

// DiffFile returns the unified diff for one path between two refs.
func DiffFile(root, fromRef, toRef, path string) (string, error) {
	return run(root, nil, "diff", "--no-color", fromRef, toRef, "--", path)
}

// FileChange is one file's change between two snapshots, with a
// human-readable type: "added", "modified", "deleted", or "renamed".
type FileChange struct {
	Type string
	Path string
}

// ChangedFilesStatus lists files that differ between two refs with
// their change type, parsed from `git diff --name-status`.
func ChangedFilesStatus(root, fromRef, toRef string) ([]FileChange, error) {
	out, err := run(root, nil, "diff", "--name-status", fromRef, toRef)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var changes []FileChange
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		path := fields[1]
		var typ string
		switch {
		case status == "A":
			typ = "added"
		case status == "D":
			typ = "deleted"
		case strings.HasPrefix(status, "R"):
			typ = "renamed"
			if len(fields) >= 3 {
				path = fields[2] // rename: old path, new path
			}
		default:
			typ = "modified"
		}
		changes = append(changes, FileChange{Type: typ, Path: path})
	}
	return changes, nil
}

// Restore checks out the given paths from ref into the working tree.
// It never uses reset --hard: only the listed paths are affected.
func Restore(root, ref string, paths []string) error {
	if len(paths) == 0 {
		return fmt.Errorf("restore requires at least one path (refusing to restore nothing)")
	}
	args := append([]string{"checkout", ref, "--"}, paths...)
	_, err := run(root, nil, args...)
	return err
}
