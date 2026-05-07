package sync

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// ErrGitNotAvailable is returned when git is not on PATH.
var ErrGitNotAvailable = errors.New("git not on PATH; required for auto_commit")

// PreCommit stages and commits all changes in syncDir before a pull or push.
// No-ops if auto_commit is false, git is not a repo, or the tree is clean.
func PreCommit(syncDir, op string, autoCommit bool) error {
	return maybeCommit(syncDir, autoCommit, "nodin: pre-"+op+" snapshot")
}

// PostCommit stages and commits all changes in syncDir after a pull or push.
func PostCommit(syncDir, op, summary string, autoCommit bool) error {
	msg := "nodin: " + op
	if summary != "" {
		msg += ": " + summary
	}
	return maybeCommit(syncDir, autoCommit, msg)
}

func maybeCommit(syncDir string, autoCommit bool, msg string) error {
	if !autoCommit {
		return nil
	}

	if _, err := exec.LookPath("git"); err != nil {
		return ErrGitNotAvailable
	}

	if !isGitRepo(syncDir) {
		return fmt.Errorf("auto_commit=true but %s is not a git repo; run 'nodin init' or set auto_commit=false", syncDir)
	}

	if clean, err := isClean(syncDir); err != nil || clean {
		return err
	}

	if err := runGit(syncDir, "add", "-A"); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if err := runGit(syncDir, "commit", "-m", msg); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// isGitRepo returns true if syncDir is inside a git repository.
func isGitRepo(syncDir string) bool {
	_, err := os.Stat(filepath.Join(syncDir, ".git"))
	return err == nil
}

// isClean returns true if the working tree has no staged or unstaged changes.
func isClean(syncDir string) (bool, error) {
	cmd := exec.Command("git", "-C", syncDir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	return len(out) == 0, nil
}

// runGit runs a git command in syncDir, returning an error on non-zero exit.
func runGit(syncDir string, args ...string) error {
	fullArgs := append([]string{"-C", syncDir}, args...)
	cmd := exec.Command("git", fullArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}
