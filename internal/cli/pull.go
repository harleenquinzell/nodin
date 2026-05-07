package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
	internalsync "github.com/harleenquinzell/nodin/internal/sync"
)

func newPullCmd() *cobra.Command {
	var dryRun bool
	var pageID string
	var since string

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Sync Notion → local (incremental)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if err := cfg.Validate(); err != nil {
				return fmt.Errorf("invalid config: %w", err)
			}

			token, err := cfg.ResolvedToken()
			if err != nil {
				return fmt.Errorf("resolve token: %w", err)
			}

			if dryRun {
				cmd.Println("dry-run: no files will be written")
				return nil
			}

			_ = pageID
			_ = since

			client := notion.NewClient(token, cfg.RPS)
			store := state.Open(cfg.SyncDir)

			if err := store.Init(); err != nil {
				return fmt.Errorf("init state: %w", err)
			}

			ctx := cmd.Context()
			report, err := internalsync.Pull(ctx, cfg, store, client)
			if err != nil {
				return err
			}

			cmd.Printf("pull: %s\n", report.Summary())
			if report.Conflicts > 0 {
				return fmt.Errorf("%d conflict(s); resolve and re-run push", report.Conflicts)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without writing files")
	cmd.Flags().StringVar(&pageID, "page", "", "pull only this page (and its descendants)")
	cmd.Flags().StringVar(&since, "since", "", "override incremental cursor (ISO 8601 timestamp)")

	return cmd
}
