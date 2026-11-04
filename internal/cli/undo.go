package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
	"agentledger/internal/store"
	"agentledger/internal/ui"
)

var undoFiles []string
var undoDryRun bool
var undoForce bool

// renameFrom extracts the original path from a rename diff
// ("rename from <old>" line), or "" if the diff has none.
func renameFrom(diff string) string {
	for _, line := range strings.Split(diff, "\n") {
		if rest, ok := strings.CutPrefix(line, "rename from "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// runUndo performs an undo and returns the number of files restored
// (0 for a dry run or a nothing-to-undo session). It is shared by
// the undo command and the web UI's undo endpoint, so both enforce
// exactly the same safety rules: validate targets first, refuse on
// post-session changes unless force is given, and never touch the
// working tree on a dry run.
func runUndo(root string, st *store.Store, sess *store.Session, files []string, dryRun, force bool, out io.Writer) (int, error) {
	actions, err := st.ListActions(sess.ID)
	if err != nil {
		return 0, err
	}
	byPath := map[string]*store.Action{}
	for _, a := range actions {
		byPath[a.Path] = a
	}
	targets := files
	if len(targets) == 0 {
		for _, a := range actions {
			targets = append(targets, a.Path)
		}
	}
	// Safety check scope: every file in either snapshot, not
	// just the undo targets. A session with no file changes
	// must still refuse if the user overwrote an unrelated
	// file afterwards — otherwise "undo last" after unrelated
	// uncommitted work would silently succeed as a no-op
	// instead of protecting that work. This check is built
	// before the nothing-to-undo early return, so that
	// return cannot bypass it.
	checkSet := map[string]bool{}
	for _, p := range targets {
		checkSet[p] = true
	}
	for _, ref := range []string{sess.StartRef, sess.EndRef} {
		files, err := gitx.ListFiles(root, ref)
		if err != nil {
			return 0, err
		}
		for _, p := range files {
			checkSet[p] = true
		}
	}
	var checkPaths []string
	for p := range checkSet {
		checkPaths = append(checkPaths, p)
	}
	sort.Strings(checkPaths)
	formatChanged := func(changed []string) string {
		if len(changed) > 20 {
			return strings.Join(changed[:20], "\n") + fmt.Sprintf("\n... and %d more file(s)", len(changed)-20)
		}
		return strings.Join(changed, "\n")
	}
	if len(targets) == 0 {
		if !dryRun && !force {
			changed, err := gitx.ChangedSinceSession(root, sess.StartRef, sess.EndRef, checkPaths)
			if err != nil {
				return 0, err
			}
			if len(changed) > 0 {
				return 0, fmt.Errorf("%w: "+UndoDirty, ui.ErrDirty, formatChanged(changed))
			}
		}
		fmt.Fprintf(out, UndoNoChanges+"\n", sess.ID)
		return 0, nil
	}
	// Validate everything before changing anything.
	for _, path := range targets {
		a, ok := byPath[path]
		if !ok {
			return 0, fmt.Errorf(UndoFileUnknown, path, sess.ID, sess.ID)
		}
		if a.Type == "renamed" && renameFrom(a.Diff) == "" {
			return 0, fmt.Errorf(UndoRenameUnknown, path)
		}
	}

	if dryRun {
		for _, path := range targets {
			a := byPath[path]
			if a.Type == "added" {
				fmt.Fprintf(out, UndoDryRunDelete+"\n", path)
			} else {
				fmt.Fprintf(out, UndoDryRunRestore+"\n", path, a.Type)
			}
		}
		fmt.Fprintf(out, UndoDryRunSummary+"\n", len(targets))
		if changed, err := gitx.ChangedSinceSession(root, sess.StartRef, sess.EndRef, checkPaths); err == nil && len(changed) > 0 {
			fmt.Fprintln(out, UndoDirtyWarning)
		}
		return 0, nil
	}

	if !force {
		changed, err := gitx.ChangedSinceSession(root, sess.StartRef, sess.EndRef, checkPaths)
		if err != nil {
			return 0, err
		}
		if len(changed) > 0 {
			return 0, fmt.Errorf("%w: "+UndoDirty, ui.ErrDirty, formatChanged(changed))
		}
	}

	for _, path := range targets {
		a := byPath[path]
		switch a.Type {
		case "added":
			full := filepath.Join(root, path)
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return 0, fmt.Errorf("delete added file %s: %w", path, err)
			}
		case "renamed":
			oldPath := renameFrom(a.Diff)
			full := filepath.Join(root, path)
			if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
				return 0, fmt.Errorf("delete renamed file %s: %w", path, err)
			}
			if err := gitx.Restore(root, sess.StartRef, []string{oldPath}); err != nil {
				return 0, fmt.Errorf("restore %s from start snapshot: %w", oldPath, err)
			}
		default: // modified, deleted, anything else: checkout from start
			if err := gitx.Restore(root, sess.StartRef, []string{path}); err != nil {
				return 0, fmt.Errorf("restore %s from start snapshot: %w", path, err)
			}
		}
	}
	// Only a full undo marks the session undone. A partial --file
	// undo leaves other files changed, so reporting the session as
	// undone would mislead: a later full undo must still be
	// possible and must be the one that marks it.
	if len(targets) == len(actions) {
		if err := st.MarkUndone(sess.ID, time.Now()); err != nil {
			return 0, err
		}
	}
	fmt.Fprintf(out, UndoSummary+"\n", sess.ID, len(targets))
	return len(targets), nil
}

var undoCmd = &cobra.Command{
	Use:   UndoUse,
	Short: UndoShort,
	Long:  UndoLong,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, st, err := openStoreForRepo()
		if err != nil {
			return err
		}
		defer st.Close()
		sess, err := resolveSession(st, args[0])
		if err != nil {
			return err
		}
		_, err = runUndo(root, st, sess, undoFiles, undoDryRun, undoForce, cmd.OutOrStdout())
		return err
	},
}

func init() {
	undoCmd.Flags().StringArrayVarP(&undoFiles, "file", "f", nil, UndoFileHelp)
	undoCmd.Flags().BoolVar(&undoDryRun, "dry-run", false, UndoDryRunHelp)
	undoCmd.Flags().BoolVar(&undoForce, "force", false, UndoForceHelp)
}
