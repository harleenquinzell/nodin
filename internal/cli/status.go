package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/state"
	internalsync "github.com/harleenquinzell/nodin/internal/sync"
)

func newStatusCmd() *cobra.Command {
	var showClean bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local vs. last-synced state",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			store := state.Open(cfg.SyncDir)
			entries, err := internalsync.Status(cfg, store)
			if err != nil {
				return fmt.Errorf("status: %w", err)
			}

			modified := 0
			var conflictedPaths []string
			deleted := 0
			for _, e := range entries {
				switch e.Status {
				case internalsync.FileModified:
					modified++
					cmd.Printf("M  %s\n", e.LocalPath)
				case internalsync.FileConflicted:
					conflictedPaths = append(conflictedPaths, e.LocalPath)
					cmd.Printf("C  %s\n", e.LocalPath)
				case internalsync.FileDeleted:
					deleted++
					cmd.Printf("D  %s\n", e.LocalPath)
				default:
					if showClean {
						cmd.Printf("   %s\n", e.LocalPath)
					}
				}
			}

			conflicted := len(conflictedPaths)
			if modified+conflicted+deleted == 0 {
				cmd.Println("nothing to push")
			} else {
				cmd.Printf("\n%d modified, %d conflicted, %d deleted\n", modified, conflicted, deleted)
			}
			printConflictHints(cmd.OutOrStdout(), conflictedPaths, func(p string) string {
				return filepath.Join(cfg.SyncDir, p)
			})
			return nil
		},
	}

	cmd.Flags().BoolVar(&showClean, "all", false, "also show unmodified files")
	return cmd
}
