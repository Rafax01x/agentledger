package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var showDiff bool
var showEvents bool

var showCmd = &cobra.Command{
	Use:   ShowUse,
	Short: ShowShort,
	Long:  ShowLong,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, st, err := openStoreForRepo()
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
		events, err := st.ListEvents(sess.ID)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		exit := "?"
		if sess.ExitCode != nil {
			exit = fmt.Sprintf("%d", *sess.ExitCode)
		}
		ended := ""
		if sess.EndedAt != nil {
			ended = sess.EndedAt.Local().Format("2006-01-02 15:04:05")
		}
		undone := ""
		if sess.UndoneAt != nil {
			undone = sess.UndoneAt.Local().Format("2006-01-02 15:04:05")
		}
		for _, f := range []struct{ k, v string }{
			{"ID", sess.ID},
			{"Agent", sess.Agent},
			{"Command", sess.Command},
			{"Cwd", sess.Cwd},
			{"Started", sess.StartedAt.Local().Format("2006-01-02 15:04:05")},
			{"Ended", ended},
			{"Duration", fmt.Sprintf("%d ms", sess.DurationMs)},
			{"Exit code", exit},
			{"Start ref", sess.StartRef},
			{"End ref", sess.EndRef},
			{"Files changed", fmt.Sprintf("%d", sess.FilesChanged)},
			{"Undone at", undone},
		} {
			fmt.Fprintf(out, ShowField+"\n", f.k, f.v)
		}
		if sess.LinesAdded != nil && sess.LinesDeleted != nil {
			fmt.Fprintf(out, ShowField+"\n", "Lines", fmt.Sprintf("+%d -%d", *sess.LinesAdded, *sess.LinesDeleted))
		}
		if sess.InputTokens != nil {
			fmt.Fprintf(out, ShowField+"\n", "Input tokens", fmt.Sprintf("%d", *sess.InputTokens))
		}
		if sess.OutputTokens != nil {
			fmt.Fprintf(out, ShowField+"\n", "Output tokens", fmt.Sprintf("%d", *sess.OutputTokens))
		}
		if sess.CostUSD != nil {
			fmt.Fprintf(out, ShowField+"\n", "Cost (USD)", fmt.Sprintf("$%.4f", *sess.CostUSD))
		}
		fmt.Fprintln(out, ShowFilesTitle)
		for _, a := range actions {
			fmt.Fprintf(out, ShowFileLine+"\n", a.Path, a.Type)
			if showDiff && a.Diff != "" {
				fmt.Fprintf(out, ShowDiffTitle+"\n%s\n", a.Path, a.Type, a.Diff)
			}
		}
		fmt.Fprintln(out, ShowOutputTitle)
		for _, e := range events {
			fmt.Fprintf(out, ShowOutputLine+"\n", e.Stream, e.Line)
		}
		return nil
	},
}

func init() {
	showCmd.Flags().BoolVar(&showDiff, "diff", false, "also print each file's diff")
	// Accepted for compatibility: show always includes the output
	// section, so --events does not change the output, it only
	// makes the intent explicit (and keeps `show last --events`
	// working as a way to inspect captured output).
	showCmd.Flags().BoolVar(&showEvents, "events", false, ShowEventsHelp)
}
