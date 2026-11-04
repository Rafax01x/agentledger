package cli

import (
	"github.com/spf13/cobra"
)

// rootCmd is the base command for agentledger. Subcommands (init, run,
// log, show, undo, export, doctor) are added in later milestones.
var rootCmd = &cobra.Command{
	Use:     AppName,
	Short:   RootShort,
	Long:    RootLong,
	Version: Version,
}

func init() {
	rootCmd.AddCommand(initCmd, doctorCmd, runCmd, logCmd, showCmd, exportCmd, undoCmd, diffCmd, cleanCmd, uiCmd)
}

// Execute runs the root command and returns any error to main,
// which maps it to a non-zero exit code.
func Execute() error {
	return rootCmd.Execute()
}
