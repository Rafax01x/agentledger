package cli

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"agentledger/internal/gitx"
)

var cleanOlderThan string
var cleanKeepLast int
var cleanDryRun bool

var ageRe = regexp.MustCompile(`^(\d+)([dwh])?$`)

// parseAge parses durations like "30d", "7d", "2w", "24h", or a bare
// number of days. It rejects anything else so a typo cannot silently
// prune everything (or nothing).
func parseAge(s string) (time.Duration, error) {
	m := ageRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf(CleanBadAge, s)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf(CleanBadAge, s)
	}
	switch m[2] {
	case "w":
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	case "h":
		return time.Duration(n) * time.Hour, nil
	default: // "d" or bare number: days
		return time.Duration(n) * 24 * time.Hour, nil
	}
}

var cleanCmd = &cobra.Command{
	Use:   CleanUse,
	Short: CleanShort,
	Long:  CleanLong,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		age, err := parseAge(cleanOlderThan)
		if err != nil {
			return err
		}
		root, st, err := openStoreForRepo()
		if err != nil {
			return err
		}
		defer st.Close()
		sessions, err := st.ListSessions(0)
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		for i, s := range sessions {
			if i < cleanKeepLast {
				keep[s.ID] = true
			}
		}
		cutoff := time.Now().Add(-age)
		type victim struct {
			id      string
			started time.Time
			bytes   int64
		}
		var victims []victim
		var storedBytes int64
		for _, s := range sessions {
			if keep[s.ID] || !s.StartedAt.Before(cutoff) {
				continue
			}
			b, err := st.SessionStoredBytes(s.ID)
			if err != nil {
				return err
			}
			victims = append(victims, victim{id: s.ID, started: s.StartedAt, bytes: b})
			storedBytes += b
		}
		out := cmd.OutOrStdout()
		if len(victims) == 0 {
			fmt.Fprintf(out, CleanNothing+"\n", cleanOlderThan, cleanKeepLast)
			return nil
		}
		if cleanDryRun {
			for _, v := range victims {
				fmt.Fprintf(out, CleanDryLine+"\n", v.id, v.started.Local().Format("2006-01-02 15:04:05"), v.bytes)
			}
			fmt.Fprintf(out, CleanDrySummary+"\n", len(victims), storedBytes)
			return nil
		}
		dbFile := dbPath(root)
		var sizeBefore int64
		if info, err := os.Stat(dbFile); err == nil {
			sizeBefore = info.Size()
		}
		for _, v := range victims {
			if err := st.DeleteSession(v.id); err != nil {
				return err
			}
			if err := gitx.DeleteSessionRefs(root, v.id); err != nil {
				return err
			}
		}
		if err := st.Vacuum(); err != nil {
			return err
		}
		var sizeAfter int64
		if info, err := os.Stat(dbFile); err == nil {
			sizeAfter = info.Size()
		}
		reclaimed := sizeBefore - sizeAfter
		if reclaimed < 0 {
			reclaimed = 0
		}
		fmt.Fprintf(out, CleanDeleted+"\n", len(victims))
		fmt.Fprintf(out, CleanReclaimed+"\n", reclaimed, storedBytes, sizeBefore, sizeAfter)
		return nil
	},
}

func init() {
	cleanCmd.Flags().StringVar(&cleanOlderThan, "older-than", "30d", CleanOlderHelp)
	cleanCmd.Flags().IntVar(&cleanKeepLast, "keep-last", 10, CleanKeepHelp)
	cleanCmd.Flags().BoolVar(&cleanDryRun, "dry-run", false, CleanDryHelp)
}
