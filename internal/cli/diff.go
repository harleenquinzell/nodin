package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/harleenquinzell/nodin/internal/config"
	"github.com/harleenquinzell/nodin/internal/state"
)

func newDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <file>",
		Short: "Show diff between local file and last snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			store := state.Open(cfg.SyncDir)
			idx, err := store.ReadIndex()
			if err != nil {
				return fmt.Errorf("read index: %w", err)
			}

			// Resolve the argument to a path relative to SyncDir.
			// Relative paths are first resolved against CWD so the command
			// works correctly regardless of which directory the user is in.
			arg := args[0]
			if !filepath.IsAbs(arg) {
				cwd, _ := os.Getwd()
				arg = filepath.Join(cwd, arg)
			}
			relPath, err := filepath.Rel(cfg.SyncDir, arg)
			if err != nil {
				relPath = arg
			}
			relPath = filepath.ToSlash(relPath)

			var notionID string
			for id, entry := range idx {
				if filepath.ToSlash(entry.LocalPath) == relPath {
					notionID = id
					break
				}
			}
			if notionID == "" {
				return fmt.Errorf("%s is not tracked (run 'nodin pull' first)", arg)
			}

			snapshot, err := store.ReadSnapshot(notionID)
			if err != nil {
				return fmt.Errorf("read snapshot: %w", err)
			}
			if snapshot == "" {
				return fmt.Errorf("no snapshot for %s; run 'nodin pull' first", arg)
			}

			absPath := filepath.Join(cfg.SyncDir, relPath)

			// If the file contains unresolved conflict markers, show a resolve hint.
			if line := firstConflictLine(absPath); line > 0 {
				printConflictHints(cmd.OutOrStdout(), []string{relPath}, func(p string) string {
					return filepath.Join(cfg.SyncDir, p)
				})
				cmd.Println()
			}

			// Write snapshot to a temp file so we can diff it.
			tmpFile, err := os.CreateTemp("", "nodin-snapshot-*")
			if err != nil {
				return fmt.Errorf("create temp file: %w", err)
			}
			defer func() { _ = os.Remove(tmpFile.Name()) }()
			if _, err := tmpFile.WriteString(snapshot); err != nil {
				_ = tmpFile.Close()
				return fmt.Errorf("write temp file: %w", err)
			}
			_ = tmpFile.Close()

			// Try git diff --no-index first; fall back to diff.
			diffOutput, diffErr := runDiff(tmpFile.Name(), absPath, relPath)
			if diffOutput != "" {
				cmd.Print(diffOutput)
			} else if diffErr != nil {
				return diffErr
			}
			return nil
		},
	}
	return cmd
}

// runDiff produces a unified diff between snapshotPath and localPath.
// It tries git diff --no-index then POSIX diff.
func runDiff(snapshotPath, localPath, label string) (string, error) {
	// Prefer git diff for colour and familiar output.
	if _, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command("git", "diff", "--no-index", "--", snapshotPath, localPath)
		out, err := cmd.Output()
		// git diff returns exit code 1 when there are differences; that is not an error.
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
				return rewriteHeader(string(out), snapshotPath, localPath, label), nil
			}
			// Fall through to diff.
		} else {
			return "", nil // no differences
		}
	}

	// Fall back to POSIX diff.
	if _, err := exec.LookPath("diff"); err == nil {
		cmd := exec.Command("diff", "-u", snapshotPath, localPath)
		out, err := cmd.Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
				return rewriteHeader(string(out), snapshotPath, localPath, label), nil
			}
			return "", fmt.Errorf("diff: %w", err)
		}
		return "", nil // no differences
	}

	return "", fmt.Errorf("neither git nor diff is available on PATH")
}

// rewriteHeader replaces the temp file path in the diff header with the label.
func rewriteHeader(raw, snapshotPath, localPath, label string) string {
	out := strings.ReplaceAll(raw, snapshotPath, "a/"+label)
	out = strings.ReplaceAll(out, localPath, "b/"+label)
	return out
}
