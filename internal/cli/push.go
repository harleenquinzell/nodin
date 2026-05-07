package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

			store := state.Open(cfg.SyncDir)
			if err := store.Init(); err != nil {
				return fmt.Errorf("init state: %w", err)
			}

			// If --page looks like a path, resolve it against CWD so the
			// command works correctly from any directory.
			resolvedPage := resolvePageFilter(pageID, cfg.SyncDir)

			if dryRun {
				entries, err := internalsync.Status(cfg, store)
				if err != nil {
					return fmt.Errorf("status: %w", err)
				}
				n := 0
				for _, e := range entries {
					if e.Status == internalsync.FileModified || e.Status == internalsync.FileDeleted {
						if resolvedPage == "" || e.NotionID == resolvedPage || e.LocalPath == resolvedPage {
							cmd.Printf("  %s  %s\n", e.Status, e.LocalPath)
							n++
						}
					}
				}
				if n == 0 {
					cmd.Println("dry-run: nothing to push")
				} else {
					cmd.Printf("dry-run: %d page(s) would be pushed\n", n)
				}
				return nil
			}

			token, err := cfg.ResolvedToken()
			if err != nil {
				return fmt.Errorf("resolve token: %w", err)
			}

			client := notion.NewClient(token, cfg.RPS)
			ctx := cmd.Context()

			pushOpts := internalsync.PushOptions{PageID: resolvedPage}
			report, err := internalsync.Push(ctx, cfg, store, client, pushOpts)
			if err != nil {
				return err
			}

			cmd.Printf("push: %s\n", report.Summary())
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "report what would change without pushing to Notion")
	cmd.Flags().StringVar(&pageID, "page", "", "push only this page (Notion ID or local path)")

	return cmd
}

// resolvePageFilter normalises a --page value so it can be matched against
// index entries regardless of the caller's working directory.
// Notion UUIDs (no path separators or extensions) are returned as-is.
// Path-like values are resolved against CWD then made relative to syncDir.
func resolvePageFilter(page, syncDir string) string {
	if page == "" {
		return ""
	}
	// Notion UUIDs contain only hex digits and dashes — no slashes or dots.
	if !strings.ContainsAny(page, "/\\.") && !filepath.IsAbs(page) {
		return page
	}
	abs := page
	if !filepath.IsAbs(page) {
		cwd, _ := os.Getwd()
		abs = filepath.Join(cwd, page)
	}
	rel, err := filepath.Rel(syncDir, abs)
	if err != nil {
		return page
	}
	return filepath.ToSlash(rel)
}
