package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
)

var diffStat bool

var diffCmd = &cobra.Command{
	Use:   DiffUse,
	Short: DiffShort,
	Long:  DiffLong,
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
		out := cmd.OutOrStdout()
		if len(actions) == 0 {
			fmt.Fprintf(out, DiffNoDiffs+"\n", sess.ID)
			return nil
		}
		if diffStat {
			var totalAdded, totalDeleted int
			for _, a := range actions {
				added, deleted := gitx.DiffStat(a.Diff)
				totalAdded += added
				totalDeleted += deleted
				fmt.Fprintf(out, DiffStatLine+"\n", a.Path, added, deleted)
			}
			fmt.Fprintf(out, DiffStatTotal+"\n", len(actions), totalAdded, totalDeleted)
			return nil
		}
		for _, a := range actions {
			fmt.Fprintf(out, DiffFileHeader+"\n", a.Path, a.Type)
			if a.Diff != "" {
				fmt.Fprintln(out, a.Diff)
			}
		}
		return nil
	},
}

func init() {
	diffCmd.Flags().BoolVar(&diffStat, "stat", false, DiffStatHelp)
}
