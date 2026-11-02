package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
	"agentledger/internal/store"
)

type check struct {
	name   string
	ok     bool
	detail string
}

var doctorCmd = &cobra.Command{
	Use:   DoctorUse,
	Short: DoctorShort,
	Long:  DoctorLong,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		var checks []check

		// git binary.
		gitPath, err := exec.LookPath("git")
		checks = append(checks, check{name: DoctorCheckGit, ok: err == nil, detail: gitPath})

		// Repository root and HEAD.
		cwd, _ := os.Getwd()
		root, err := gitx.FindRoot(cwd)
		checks = append(checks, check{name: DoctorCheckRepo, ok: err == nil, detail: root})
		head := ""
		if err == nil {
			head, err = gitx.Head(root)
		}
		checks = append(checks, check{name: DoctorCheckHead, ok: err == nil, detail: head})

		// .agentledger directory and database.
		dirOK := false
		dbOK := false
		if err == nil {
			if info, err := os.Stat(agentledgerDir(root)); err == nil && info.IsDir() {
				dirOK = true
			}
			if _, err := os.Stat(dbPath(root)); err == nil {
				st, err := store.Open(dbPath(root))
				if err == nil {
					dbOK = true
					st.Close()
				}
			}
		}
		checks = append(checks, check{name: DoctorCheckDir, ok: dirOK})
		checks = append(checks, check{name: DoctorCheckDB, ok: dbOK})

		failures := 0
		for _, c := range checks {
			status := DoctorOK
			if !c.ok {
				status = DoctorFail
				failures++
			}
			if c.detail != "" {
				fmt.Fprintf(out, "%s: %s (%s)\n", c.name, status, c.detail)
			} else {
				fmt.Fprintf(out, "%s: %s\n", c.name, status)
			}
		}

		// Working tree status is informational only, never a failure.
		if err == nil {
			status, err := gitx.Status(root)
			if err == nil {
				if status == "" {
					fmt.Fprintf(out, "%s: %s\n", DoctorWorkingTree, DoctorWorkingClean)
				} else {
					fmt.Fprintf(out, "%s: %s\n", DoctorWorkingTree, DoctorWorkingDirty)
				}
			}
		} else if !dirOK {
			fmt.Fprintln(out, DoctorNotInitialized)
		}

		if failures > 0 {
			return fmt.Errorf(DoctorFailed, failures)
		}
		fmt.Fprintln(out, DoctorAllOK)
		return nil
	},
}
