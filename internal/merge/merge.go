package merge

import (
	"fmt"
	"os"
	"os/exec"
)

// Result holds the outcome of a three-way merge.
type Result struct {
	Content   string
	Conflicts bool
}

// ThreeWay merges local and remote with base as the common ancestor.
// It shells out to git merge-file -p --diff3.
// Returns Result.Conflicts=true if conflict markers are present (exit code 1).
// Returns an error for exit codes >= 2 or if git is not available.
func ThreeWay(base, local, remote string) (Result, error) {
	baseF, err := writeTmp("nodin-base-*", base)
	if err != nil {
		return Result{}, fmt.Errorf("write base temp: %w", err)
	}
	defer func() { _ = os.Remove(baseF) }()

	localF, err := writeTmp("nodin-local-*", local)
	if err != nil {
		return Result{}, fmt.Errorf("write local temp: %w", err)
	}
	defer func() { _ = os.Remove(localF) }()

	remoteF, err := writeTmp("nodin-remote-*", remote)
	if err != nil {
		return Result{}, fmt.Errorf("write remote temp: %w", err)
	}
	defer func() { _ = os.Remove(remoteF) }()

	// git merge-file -p --diff3 <local> <base> <remote>
	// -p: output to stdout instead of modifying localF
	// --diff3: include diff3-style conflict markers with base content
	cmd := exec.Command("git", "merge-file", "-p", "--diff3", localF, baseF, remoteF)
	out, err := cmd.Output()

	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			return Result{}, fmt.Errorf("git merge-file: %w", err)
		}
		if exitErr.ExitCode() == 1 {
			return Result{Content: string(out), Conflicts: true}, nil
		}
		return Result{}, fmt.Errorf("git merge-file exit %d: %s", exitErr.ExitCode(), exitErr.Stderr)
	}

	return Result{Content: string(out), Conflicts: false}, nil
}

func writeTmp(pattern, content string) (string, error) {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", err
	}
	_, writeErr := f.WriteString(content)
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(f.Name())
		return "", writeErr
	}
	if closeErr != nil {
		_ = os.Remove(f.Name())
		return "", closeErr
	}
	return f.Name(), nil
}
