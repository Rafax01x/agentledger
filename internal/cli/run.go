package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
	"agentledger/internal/session"
	"agentledger/internal/store"
)

// ExitError carries a child process exit code through cobra to main,
// which exits with that code instead of the generic 1. Its Error is
// only used if it is printed somewhere else; the run command itself
// stays silent on a child failure (the summary line already reported it).
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf(RunChildFailed, e.Code)
}

var runCmd = &cobra.Command{
	Use:           RunUse,
	Short:         RunShort,
	Long:          RunLong,
	Args:          cobra.MinimumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("%s", RunNoCommand)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
		root, err := gitx.FindRoot(cwd)
		if err != nil {
			return fmt.Errorf(RunNotGitRepo, cwd)
		}
		db := dbPath(root)
		if _, err := os.Stat(db); err != nil {
			return fmt.Errorf(RunNotInitialized, agentledgerDir(root))
		}
		st, err := store.Open(db)
		if err != nil {
			return err
		}
		defer st.Close()

		agent := filepath.Base(args[0])
		res, err := session.Run(session.Config{
			Root: root, Cwd: cwd, Agent: agent,
			Command: args, Env: os.Environ(),
			Stdout: cmd.OutOrStdout(), Stderr: cmd.ErrOrStderr(),
			Store: st,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), RunSummary+"\n",
			res.Session.ID, res.ExitCode, res.Session.FilesChanged)
		if res.ExitCode != 0 {
			return &ExitError{Code: res.ExitCode}
		}
		return nil
	},
}
