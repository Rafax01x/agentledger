package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"agentledger/internal/report"
)

var exportFormat string

var exportCmd = &cobra.Command{
	Use:   ExportUse,
	Short: ExportShort,
	Long:  ExportLong,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if exportFormat != "markdown" && exportFormat != "jsonl" {
			return fmt.Errorf(ExportBadFormat, exportFormat)
		}
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
		if exportFormat == "markdown" {
			return report.WriteMarkdown(out, sess, actions, events)
		}
		return report.WriteJSONL(out, sess, actions, events)
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "", "export format: markdown or jsonl (required)")
}
