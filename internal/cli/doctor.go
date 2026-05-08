package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check nodin configuration and connectivity",
		Long: `doctor runs a series of checks to verify that nodin is correctly
configured and can reach your Notion workspace.

Exit code 0 means all checks passed. Exit code 2 means one or more
checks failed; the output describes which ones and why.`,
		RunE: runDoctor,
	}
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	allOk := true
	check := func(label string, fn func() error) {
		if err := fn(); err != nil {
			fmt.Printf("  [fail] %s: %v\n", label, err)
			allOk = false
		} else {
			fmt.Printf("  [ok]   %s\n", label)
		}
	}

	fmt.Println("nodin doctor")
	fmt.Println()

	// Resolve the config file path the same way Load does, so we can check
	// its permissions below.
	resolvedCfgPath := cfgPath
	if resolvedCfgPath == "" {
		resolvedCfgPath = config.Discover()
	}

	// 1. Config
	var c *config.Config
	var cfgErr error
	check("config loads and validates", func() error {
		c, cfgErr = config.Load(cfgPath)
		if cfgErr != nil {
			return cfgErr
		}
		return c.Validate()
	})
	if !allOk {
		fmt.Println()
		fmt.Println("Fix configuration errors before running other checks.")
		return fmt.Errorf("doctor: checks failed")
	}

	// 2. Config file permissions — warn if inline token is world/group readable.
	check("config file permissions", func() error {
		if c.Token == "" {
			return nil // token comes from env or token_file; skip
		}
		info, err := os.Stat(resolvedCfgPath)
		if err != nil {
			return nil // env-only config; no file to check
		}
		if perm := info.Mode().Perm(); perm&0077 != 0 {
			return fmt.Errorf("%s is %04o; should be 0600 (token readable by others)",
				filepath.Base(resolvedCfgPath), perm)
		}
		return nil
	})

	token, _ := c.ResolvedToken()
	client := notion.NewClient(token, c.RPS)

	// 3. git (only if AutoCommit)
	if c.AutoCommit {
		check("git is available", func() error {
			_, err := exec.LookPath("git")
			if err != nil {
				return fmt.Errorf("git not found on PATH; install git or set auto_commit=false")
			}
			return nil
		})
	}

	// 4. Token authenticates
	check("token authenticates", func() error {
		_, err := client.Search(cmd.Context(), notion.SearchOpts{Limit: 1})
		return err
	})

	// 5. Root page accessible
	var rootPage *notion.Page
	check("root page accessible", func() error {
		var err error
		rootPage, err = client.GetPage(cmd.Context(), c.RootPageID)
		return err
	})

	// 6. Root is a page, not a database
	if rootPage != nil {
		check("root is a page (not a database)", func() error {
			if rootPage.Object != "page" {
				return fmt.Errorf("got object=%q; the root must be a page, not a database", rootPage.Object)
			}
			return nil
		})
	}

	// 7. SyncDir (if set)
	if c.SyncDir != "" {
		check("sync_dir exists and is writable", func() error {
			if _, err := os.Stat(c.SyncDir); err != nil {
				return fmt.Errorf("%s does not exist", c.SyncDir)
			}
			tmp, err := os.CreateTemp(c.SyncDir, ".nodin-check-*")
			if err != nil {
				return fmt.Errorf("not writable: %w", err)
			}
			tmp.Close()
			os.Remove(tmp.Name())
			return nil
		})

		if c.AutoCommit {
			check("sync_dir is a git repository", func() error {
				if _, err := os.Stat(filepath.Join(c.SyncDir, ".git")); err != nil {
					return fmt.Errorf("no .git/ in %s; run 'nodin init' or set auto_commit=false", c.SyncDir)
				}
				return nil
			})
		}

		check(".nodin/ is accessible with safe permissions", func() error {
			nodinPath := filepath.Join(c.SyncDir, ".nodin")
			info, err := os.Stat(nodinPath)
			if os.IsNotExist(err) {
				return os.MkdirAll(nodinPath, 0700)
			}
			if err != nil {
				return err
			}
			if perm := info.Mode().Perm(); perm&0077 != 0 {
				return fmt.Errorf(".nodin/ is %04o; should be 0700 (snapshots readable by others)", perm)
			}
			return nil
		})
	}

	fmt.Println()
	if allOk {
		fmt.Println("All checks passed.")
		return nil
	}
	return fmt.Errorf("doctor: checks failed")
}
