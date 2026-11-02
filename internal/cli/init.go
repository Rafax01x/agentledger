package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
	"agentledger/internal/store"
)

// defaultConfig is written to .agentledger/config.toml by init when
// no config file exists yet. v0.1 reads no options from it; the
// file exists so future versions can pick up defaults without a
// new init, and so `init` verifiably creates the expected layout.
const defaultConfig = `# agentledger configuration
# Created by ` + "`agentledger init`" + `. v0.1 does not read any options
# from this file yet; it exists so future versions can pick up
# defaults without a new init. See docs/limitations.md.
# (No options are defined in v0.1.)
`

var initCmd = &cobra.Command{
	Use:   InitUse,
	Short: InitShort,
	Long:  InitLong,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
		root, err := gitx.FindRoot(cwd)
		if err != nil {
			return fmt.Errorf(InitNotGitRepo, cwd)
		}
		dir := agentledgerDir(root)
		if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o755); err != nil {
			return fmt.Errorf("create %s: %w (check directory permissions)", dir, err)
		}
		configPath := filepath.Join(dir, "config.toml")
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			if err := os.WriteFile(configPath, []byte(defaultConfig), 0o644); err != nil {
				return fmt.Errorf("create %s: %w (check directory permissions)", configPath, err)
			}
		}
		db := dbPath(root)
		already := false
		if _, err := os.Stat(db); err == nil {
			already = true
		}
		st, err := store.Open(db)
		if err != nil {
			return err
		}
		if err := st.Close(); err != nil {
			return fmt.Errorf("close database %s: %w", db, err)
		}
		if already {
			fmt.Fprintf(cmd.OutOrStdout(), InitAlreadyInitialized+"\n", dir)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), InitInitialized+"\n", dir)
		}
		return nil
	},
}
