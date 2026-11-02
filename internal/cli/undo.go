package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
	"agentledger/internal/store"
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
		actions, err := st.ListActions(sess.ID)
		if err != nil {
			return err
		}
		byPath := map[string]*store.Action{}
		for _, a := range actions {
			byPath[a.Path] = a
		}
		targets := undoFiles
		if len(targets) == 0 {
			for _, a := range actions {
				targets = append(targets, a.Path)
			}
		}
		out := cmd.OutOrStdout()
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
				return err
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
			if !undoDryRun && !undoForce {
				changed, err := gitx.ChangedSinceSession(root, sess.StartRef, sess.EndRef, checkPaths)
				if err != nil {
					return err
				}
				if len(changed) > 0 {
					return fmt.Errorf(UndoDirty, formatChanged(changed))
				}
			}
			fmt.Fprintf(out, UndoNoChanges+"\n", sess.ID)
			return nil
		}
		// Validate everything before changing anything.
		for _, path := range targets {
			a, ok := byPath[path]
			if !ok {
				return fmt.Errorf(UndoFileUnknown, path, sess.ID, sess.ID)
			}
			if a.Type == "renamed" && renameFrom(a.Diff) == "" {
				return fmt.Errorf(UndoRenameUnknown, path)
			}
		}

		if undoDryRun {
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
			return nil
		}

		if !undoForce {
			changed, err := gitx.ChangedSinceSession(root, sess.StartRef, sess.EndRef, checkPaths)
			if err != nil {
				return err
			}
			if len(changed) > 0 {
				return fmt.Errorf(UndoDirty, formatChanged(changed))
			}
		}

		for _, path := range targets {
			a := byPath[path]
			switch a.Type {
			case "added":
				full := filepath.Join(root, path)
				if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("delete added file %s: %w", path, err)
				}
			case "renamed":
				oldPath := renameFrom(a.Diff)
				full := filepath.Join(root, path)
				if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("delete renamed file %s: %w", path, err)
				}
				if err := gitx.Restore(root, sess.StartRef, []string{oldPath}); err != nil {
					return fmt.Errorf("restore %s from start snapshot: %w", oldPath, err)
				}
			default: // modified, deleted, anything else: checkout from start
				if err := gitx.Restore(root, sess.StartRef, []string{path}); err != nil {
					return fmt.Errorf("restore %s from start snapshot: %w", path, err)
				}
			}
		}
		if err := st.MarkUndone(sess.ID, time.Now()); err != nil {
			return err
		}
		fmt.Fprintf(out, UndoSummary+"\n", sess.ID, len(targets))
		return nil
	},
}

func init() {
	undoCmd.Flags().StringArrayVar(&undoFiles, "file", nil, UndoFileHelp)
	undoCmd.Flags().BoolVar(&undoDryRun, "dry-run", false, UndoDryRunHelp)
	undoCmd.Flags().BoolVar(&undoForce, "force", false, UndoForceHelp)
}
