//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harleenquinzell/nodin/internal/notion"
)

// nodinBin is the path to the freshly-built nodin binary, set once in TestMain.
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

// run executes nodin with the given arguments from the current working directory.
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

// runIn executes nodin from dir with the given arguments.
func runIn(t *testing.T, dir string, args ...string) (output string, exitCode int) {
	t.Helper()
	cmd := exec.Command(nodinBin, args...)
	cmd.Dir = dir
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

// e2eCredentials returns the test token and root page ID, skipping the test
// if the required env vars are not set.
func e2eCredentials(t *testing.T) (token, rootPageID string) {
	t.Helper()
	token = os.Getenv("NODIN_TEST_TOKEN")
	rootPageID = os.Getenv("NODIN_TEST_PAGE_ID")
	if token == "" || rootPageID == "" {
		t.Skip("NODIN_TEST_TOKEN / NODIN_TEST_PAGE_ID not set; skipping e2e test")
	}
	return token, rootPageID
}

// initWorkspace runs "nodin init" in dir, feeding token and rootPageID via stdin.
func initWorkspace(t *testing.T, dir, token, rootPageID string) {
	t.Helper()
	cmd := exec.Command(nodinBin, "init")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(token + "\n" + rootPageID + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("nodin init failed: %v\noutput:\n%s", err, out)
	}
}

// --- Tests that run without real credentials ---

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

// --- Tests that require real credentials (skip if env not set) ---

// TestE2E_InitCreatesFiles verifies that "nodin init" scaffolds the expected
// files and directories in the workspace.
func TestE2E_InitCreatesFiles(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()

	initWorkspace(t, dir, token, rootPageID)

	for _, rel := range []string{".nodin.toml", ".nodin", ".gitignore"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s not created by init: %v", rel, err)
		}
	}
	// git init runs automatically when git is available
	if _, err := exec.LookPath("git"); err == nil {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Errorf(".git/ not created by init: %v", err)
		}
	}
}

// TestE2E_Doctor_Pass initializes a workspace and verifies that "nodin doctor"
// exits 0 with all checks passing.
func TestE2E_Doctor_Pass(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()

	initWorkspace(t, dir, token, rootPageID)

	out, code := runIn(t, dir, "doctor")
	if code != 0 {
		t.Errorf("doctor exited %d, want 0\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "All checks passed") {
		t.Errorf("doctor output does not say 'All checks passed':\n%s", out)
	}
}

// TestE2E_PullCreatesMarkdown initializes a workspace, pulls, and verifies
// that at least one markdown file was written to disk.
func TestE2E_PullCreatesMarkdown(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()

	initWorkspace(t, dir, token, rootPageID)

	out, code := runIn(t, dir, "pull")
	if code != 0 {
		t.Fatalf("pull exited %d\noutput:\n%s", code, out)
	}

	var found bool
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") {
			found = true
		}
		return nil
	})
	if !found {
		t.Error("no .md files found in sync dir after pull")
	}
}

// TestE2E_PushConflict_ExitCode creates a conflict scenario (local edit +
// concurrent remote edit on the same content), pushes, and asserts exit code 1
// with conflict markers written to the file.
func TestE2E_PushConflict_ExitCode(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()
	client := notion.NewClient(token, 3)
	ctx := context.Background()

	// Create a temporary test page under the root.
	title := fmt.Sprintf("nodin-e2e-conflict-%d", rand.Int63n(1_000_000_000))
	page, err := client.CreatePage(ctx, rootPageID, title)
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	t.Cleanup(func() { _ = client.ArchivePage(context.Background(), page.ID) })

	// Seed with a bold paragraph — bold triggers anchor emission on pull,
	// so push can match the block by ID and the conflict guard fires.
	seed := []notion.Block{{
		Type: "paragraph",
		Content: &notion.ParagraphContent{
			RichText: []notion.RichText{
				notion.NewFormattedRichText("e2e conflict base", notion.Annotations{Bold: true}, ""),
			},
		},
	}}
	seededBlocks, err := client.AppendBlocks(ctx, page.ID, seed, "")
	if err != nil {
		t.Fatalf("AppendBlocks: %v", err)
	}

	initWorkspace(t, dir, token, rootPageID)

	// Pull just this page.
	if out, code := runIn(t, dir, "pull", "--page", page.ID); code != 0 {
		t.Fatalf("pull exited %d\noutput:\n%s", code, out)
	}

	// Locate the local file via the state index.
	type indexEntry struct {
		LocalPath string `json:"local_path"`
	}
	var idx map[string]indexEntry
	idxData, err := os.ReadFile(filepath.Join(dir, ".nodin", "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := json.Unmarshal(idxData, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	entry, ok := idx[page.ID]
	if !ok {
		t.Fatalf("page %s not in index after pull", page.ID)
	}
	mdFile := filepath.Join(dir, entry.LocalPath)

	// Edit locally.
	content, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	localContent := strings.ReplaceAll(string(content), "e2e conflict base", "local e2e edit")
	if err := os.WriteFile(mdFile, []byte(localContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Wait so Notion's timestamp advances past the pull's LastSync.
	time.Sleep(1200 * time.Millisecond)

	// Update the same block via API (remote edit on the same line).
	remoteBlock := notion.Block{
		ID:   seededBlocks[0].ID,
		Type: "paragraph",
		Content: &notion.ParagraphContent{
			RichText: []notion.RichText{
				notion.NewFormattedRichText("remote e2e edit", notion.Annotations{Bold: true}, ""),
			},
		},
	}
	if err := client.UpdateBlock(ctx, seededBlocks[0].ID, remoteBlock); err != nil {
		t.Fatalf("UpdateBlock: %v", err)
	}

	// Push: conflict → exit 1.
	out, code := runIn(t, dir, "push", "--page", page.ID)
	if code != 1 {
		t.Errorf("push exit code = %d, want 1 (conflicts)\noutput:\n%s", code, out)
	}

	// Conflict markers must be written to the local file.
	written, err := os.ReadFile(mdFile)
	if err != nil {
		t.Fatalf("ReadFile after push: %v", err)
	}
	if !strings.Contains(string(written), "<<<<<<<") {
		t.Errorf("conflict markers not in file after push:\n%s", written)
	}
}

// TestE2E_Status initializes a workspace, pulls, then verifies that "nodin status"
// shows "modified" after a local edit.
func TestE2E_Status(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()

	initWorkspace(t, dir, token, rootPageID)

	if out, code := runIn(t, dir, "pull"); code != 0 {
		t.Fatalf("pull exited %d\noutput:\n%s", code, out)
	}

	// Find a tracked .md file.
	var mdFile string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".md") && mdFile == "" {
			mdFile = path
		}
		return nil
	})
	if mdFile == "" {
		t.Skip("no .md file pulled; cannot test status")
	}

	// Before edit: status should say nothing to push.
	out, code := runIn(t, dir, "status")
	if code != 0 {
		t.Fatalf("status exited %d before edit\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "nothing to push") {
		t.Errorf("expected 'nothing to push' before edit, got:\n%s", out)
	}

	// Append a line to the file.
	f, err := os.OpenFile(mdFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, _ = fmt.Fprintln(f, "\nlocal edit for status test")
	f.Close()

	// After edit: status should show the file as modified.
	out, code = runIn(t, dir, "status")
	if code != 0 {
		t.Fatalf("status exited %d after edit\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "M ") {
		t.Errorf("expected 'M ' in status output after edit, got:\n%s", out)
	}
}
