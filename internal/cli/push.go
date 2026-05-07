package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
	internalsync "github.com/harleenquinzell/nodin/internal/sync"
)

func newPushCmd() *cobra.Command {
	var dryRun bool
	var pageID string

	cmd := &cobra.Command{
		Use:   "push",
		Short: "Sync local → Notion (incremental)",
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
				cmd.Println("dry-run: no changes will be pushed to Notion")
				return nil
			}

			_ = pageID

			client := notion.NewClient(token, cfg.RPS)
			store := state.Open(cfg.SyncDir)

			if err := store.Init(); err != nil {
				return fmt.Errorf("init state: %w", err)
			}

			ctx := cmd.Context()
			report, err := internalsync.Push(ctx, cfg, store, client)
			if err != nil {
				return err
			}

			cmd.Printf("push: %s\n", report.Summary())
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without pushing to Notion")
	cmd.Flags().StringVar(&pageID, "page", "", "push only this page")

	return cmd
}
