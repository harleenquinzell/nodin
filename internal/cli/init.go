package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/notion"
	"github.com/harleenquinzell/nodin/internal/state"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new nodin workspace",
		Long: `init walks you through setting up nodin for the first time.

It prompts for your Notion integration token, the root page ID to sync
under, and a local directory to store markdown files. It then validates
both the token and page, writes the config file, scaffolds the sync
directory, and optionally runs git init.`,
		RunE: runInit,
	}
}

func runInit(cmd *cobra.Command, _ []string) error {
	r := bufio.NewReader(os.Stdin)

	fmt.Println("nodin init")
	fmt.Println()

	// token
	fmt.Print("Notion integration token (from notion.so/my-integrations): ")
	token, err := readLine(r)
	if err != nil {
		return fmt.Errorf("read token: %w", err)
	}
	if token == "" {
		return fmt.Errorf("token is required")
	}

	client := notion.NewClient(token, 3)
	fmt.Print("Validating token... ")
	if _, err := client.Search(cmd.Context(), notion.SearchOpts{Limit: 1}); err != nil {
		fmt.Println("failed")
		return fmt.Errorf("token validation: %w", err)
	}
	fmt.Println("ok")

	// root page
	fmt.Print("Root Notion page ID (the page nodin will sync under): ")
	rootPageID, err := readLine(r)
	if err != nil {
		return fmt.Errorf("read root page ID: %w", err)
	}
	if rootPageID == "" {
		return fmt.Errorf("root page ID is required")
	}

	fmt.Print("Validating root page... ")
	page, err := client.GetPage(cmd.Context(), rootPageID)
	if err != nil {
		fmt.Println("failed")
		return fmt.Errorf("access root page: %w", err)
	}
	fmt.Printf("ok (%q)\n", page.Title())

	// sync dir
	home, _ := os.UserHomeDir()
	defaultSyncDir := filepath.Join(home, "notion")
	fmt.Printf("Local sync directory [%s]: ", defaultSyncDir)
	syncDir, err := readLine(r)
	if err != nil {
		return fmt.Errorf("read sync dir: %w", err)
	}
	if syncDir == "" {
		syncDir = defaultSyncDir
	}
	if !filepath.IsAbs(syncDir) {
		abs, err := filepath.Abs(syncDir)
		if err != nil {
			return fmt.Errorf("resolve sync dir: %w", err)
		}
		syncDir = abs
	}

	// scaffold sync dir
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		return fmt.Errorf("create sync dir: %w", err)
	}

	store := state.Open(syncDir)
	if err := store.Init(); err != nil {
		return err
	}

	// git init
	if _, err := exec.LookPath("git"); err == nil {
		if _, err := os.Stat(filepath.Join(syncDir, ".git")); os.IsNotExist(err) {
			cmd := exec.Command("git", "-C", syncDir, "init")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git init failed: %v\n", err)
			}
		}
	}

	// .gitignore
	if err := ensureGitignore(syncDir); err != nil {
		return err
	}

	// config
	c := &config.Config{
		Token:          token,
		RootPageID:     rootPageID,
		SyncDir:        syncDir,
		RPS:            3,
		Concurrency:    4,
		AutoCommit:     true,
		DownloadAssets: true,
	}
	destCfg := config.DefaultPath()
	if err := config.Write(destCfg, c); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("Config written to %s\n", destCfg)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  nodin doctor     — verify everything is working\n")
	fmt.Printf("  nodin pull       — pull your Notion pages to %s\n", syncDir)
	return nil
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	return strings.TrimSpace(line), err
}

func ensureGitignore(syncDir string) error {
	path := filepath.Join(syncDir, ".gitignore")
	const nodinRules = "\n# nodin\n.nodin/\nassets/\n"

	// Append to existing file, or create a new one.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("write .gitignore: %w", err)
	}
	defer f.Close()

	// Only append if the .nodin/ rule is not already present.
	existing, _ := os.ReadFile(path)
	if !strings.Contains(string(existing), ".nodin/") {
		_, err = fmt.Fprint(f, nodinRules)
	}
	return err
}
