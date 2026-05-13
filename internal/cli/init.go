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

	// sync dir is always the current directory
	syncDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	fmt.Println("nodin init")
	fmt.Printf("Workspace directory: %s\n", syncDir)
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

	// scaffold sync dir
	store := state.Open(syncDir)
	if err := store.Init(); err != nil {
		return err
	}

	// git init
	if _, err := exec.LookPath("git"); err == nil {
		if _, err := os.Stat(filepath.Join(syncDir, ".git")); os.IsNotExist(err) {
			gitCmd := exec.Command("git", "-C", syncDir, "init")
			gitCmd.Stdout = os.Stdout
			gitCmd.Stderr = os.Stderr
			if err := gitCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: git init failed: %v\n", err)
			}
		}
	}

	// .gitignore
	if err := ensureGitignore(syncDir); err != nil {
		return err
	}

	// write .nodin.toml in the current directory (sync_dir is intentionally
	// omitted — it defaults to the directory containing the config file)
	c := &config.Config{
		Token:          token,
		RootPageID:     rootPageID,
		RPS:            3,
		Concurrency:    4,
		AutoCommit:     true,
		DownloadAssets: true,
	}
	destCfg := filepath.Join(syncDir, config.LocalConfigName)
	if err := config.Write(destCfg, c); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("Config written to %s\n", destCfg)
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  nodin doctor   — verify everything is working\n")
	fmt.Printf("  nodin pull     — pull your Notion pages into %s\n", syncDir)
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
	defer func() { _ = f.Close() }()

	// Only append if the .nodin/ rule is not already present.
	existing, _ := os.ReadFile(path)
	if !strings.Contains(string(existing), ".nodin/") {
		_, err = fmt.Fprint(f, nodinRules)
	}
	return err
}
