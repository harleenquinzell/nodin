//go:build e2e

package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// nodинBin returns the path to a freshly-built nodin binary.
// It is built once per test run and cached in t.TempDir().
var nodinBin string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "nodin-e2e-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "nodin")
	cmd := exec.Command("go", "build", "-o", bin, "../cmd/nodin")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build failed: " + err.Error())
	}
	nodinBin = bin

	os.Exit(m.Run())
}

// run executes nodin with the given arguments and returns stdout+stderr and exit code.
func run(t *testing.T, args ...string) (output string, exitCode int) {
	t.Helper()
	cmd := exec.Command(nodinBin, args...)
	out, err := cmd.CombinedOutput()
	output = string(out)
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			t.Fatalf("exec error: %v", err)
		}
	}
	return output, exitCode
}

func TestE2E_VersionFlag(t *testing.T) {
	out, code := run(t, "--version")
	if code != 0 {
		t.Errorf("exit code = %d, want 0; output: %s", code, out)
	}
	if !strings.Contains(out, "nodin") {
		t.Errorf("--version output doesn't mention nodin: %s", out)
	}
}

func TestE2E_Doctor_FailNoToken(t *testing.T) {
	// Run with no config and no env vars — token check should fail.
	cmd := exec.Command(nodinBin, "doctor")
	cmd.Env = []string{"HOME=" + t.TempDir()} // empty home → no config file
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if exitCode == 0 {
		t.Errorf("expected non-zero exit when token is absent, got 0; output: %s", out)
	}
	if strings.Contains(string(out), "secret_") || strings.Contains(string(out), "ntn_") {
		t.Errorf("doctor output contains a raw token: %s", out)
	}
}

func TestE2E_DryRun_Pull(t *testing.T) {
	// --dry-run should print a message and exit 0 without writing any files.
	dir := t.TempDir()
	cmd := exec.Command(nodinBin, "pull", "--dry-run",
		"--config", filepath.Join(dir, "config.toml"))
	cmd.Env = append(os.Environ(),
		"NODIN_TOKEN=secret_fake",
		"NODIN_ROOT_PAGE_ID=3589c940028481d3b435fcf079d89792",
		"NODIN_SYNC_DIR="+dir,
	)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if exitCode != 0 {
		t.Errorf("dry-run exited %d; output: %s", exitCode, out)
	}
	if !strings.Contains(string(out), "dry-run") {
		t.Errorf("dry-run output missing 'dry-run': %s", out)
	}
}

func TestE2E_DryRun_Push(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(nodinBin, "push", "--dry-run",
		"--config", filepath.Join(dir, "config.toml"))
	cmd.Env = append(os.Environ(),
		"NODIN_TOKEN=secret_fake",
		"NODIN_ROOT_PAGE_ID=3589c940028481d3b435fcf079d89792",
		"NODIN_SYNC_DIR="+dir,
	)
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		}
	}
	if exitCode != 0 {
		t.Errorf("dry-run exited %d; output: %s", exitCode, out)
	}
	if !strings.Contains(string(out), "dry-run") {
		t.Errorf("dry-run output missing 'dry-run': %s", out)
	}
}
