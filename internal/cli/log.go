package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var logLimit int

var logCmd = &cobra.Command{
	Use:   LogUse,
	Short: LogShort,
	Long:  LogLong,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, st, err := openStoreForRepo()
		if err != nil {
			return err
		}
		defer st.Close()
		sessions, err := st.ListSessions(logLimit)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if len(sessions) == 0 {
			fmt.Fprintln(out, NoSessions)
			return nil
		}
		for _, s := range sessions {
			exit := "?"
			if s.ExitCode != nil {
				exit = fmt.Sprintf("%d", *s.ExitCode)
			}
			undone := ""
			if s.UndoneAt != nil {
				undone = ShowUndone
			}
			fmt.Fprintf(out, LogLine+"\n",
				s.ID, s.StartedAt.Local().Format("2006-01-02 15:04:05"),
				s.Agent, s.Command, exit, s.FilesChanged, undone)
		}
		return nil
	},
}

func init() {
	logCmd.Flags().IntVar(&logLimit, "limit", 20, "maximum sessions to list (0 = all)")
}
