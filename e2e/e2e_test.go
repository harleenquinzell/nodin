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

	// Find a tracked .md file. Skip .nodin/ so we don't pick up snapshots.
	var mdFile string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == ".nodin" {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(path, ".md") && mdFile == "" {
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

// TestE2E_PushCreatesNewPage covers the full create-locally-then-push flow via
// the CLI: init → write a brand-new .md file at pages/<slug>/<slug>.md → push →
// re-pull and verify the file content matches what's now in Notion.
func TestE2E_PushCreatesNewPage(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()
	client := notion.NewClient(token, 3)

	initWorkspace(t, dir, token, rootPageID)

	// Create a brand-new page locally — no frontmatter, just an H1 + body.
	slug := fmt.Sprintf("nodin-e2e-create-%d", rand.Int63n(1_000_000_000))
	pageDir := filepath.Join(dir, "pages", slug)
	if err := os.MkdirAll(pageDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mdPath := filepath.Join(pageDir, slug+".md")
	const bodyText = "First paragraph from e2e create test."
	const expectedTitle = "e2e create heading"
	initial := "# " + expectedTitle + "\n\n" + bodyText + "\n"
	if err := os.WriteFile(mdPath, []byte(initial), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Push.
	out, code := runIn(t, dir, "push")
	if code != 0 {
		t.Fatalf("push exited %d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "1 created") {
		t.Errorf("push output missing '1 created':\n%s", out)
	}

	// Locate the index entry for the new page so we can clean it up and verify
	// it via the Notion API.
	type indexEntry struct {
		LocalPath string `json:"local_path"`
		Type      string `json:"type"`
	}
	var idx map[string]indexEntry
	idxData, err := os.ReadFile(filepath.Join(dir, ".nodin", "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := json.Unmarshal(idxData, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	relPath := filepath.ToSlash(filepath.Join("pages", slug, slug+".md"))
	var newID string
	for id, e := range idx {
		if e.Type == "page" && e.LocalPath == relPath {
			newID = id
			break
		}
	}
	if newID == "" {
		t.Fatalf("no index entry created for %s; index:\n%s", relPath, idxData)
	}
	t.Cleanup(func() { _ = client.ArchivePage(context.Background(), newID) })

	// Verify the page exists in Notion with the right title and body.
	gotPage, err := client.GetPage(context.Background(), newID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if gotPage.Title() != expectedTitle {
		t.Errorf("Notion page title = %q, want %q", gotPage.Title(), expectedTitle)
	}

	// Pull and verify the local file still exists at the user's path with the
	// expected content (frontmatter + body).
	if out, code := runIn(t, dir, "pull", "--page", newID); code != 0 {
		t.Fatalf("pull exited %d\noutput:\n%s", code, out)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Errorf("file %s missing after re-pull: %v", relPath, err)
	}
	pulled, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("ReadFile after pull: %v", err)
	}
	pulledStr := string(pulled)
	if !strings.Contains(pulledStr, bodyText) {
		t.Errorf("pulled file missing original body %q:\n%s", bodyText, pulledStr)
	}
	if !strings.Contains(pulledStr, "title: "+expectedTitle) {
		t.Errorf("pulled file missing 'title: %s' frontmatter:\n%s", expectedTitle, pulledStr)
	}

	// Now edit locally and push again — this should go through the regular
	// update path (not the create path). Verify Notion picks up the change.
	const updateLine = "Second paragraph added by e2e test."
	if err := os.WriteFile(mdPath, []byte(pulledStr+"\n"+updateLine+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile (edit): %v", err)
	}
	out, code = runIn(t, dir, "push")
	if code != 0 {
		t.Fatalf("second push exited %d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "1 pushed") {
		t.Errorf("second push output missing '1 pushed' (regular update path):\n%s", out)
	}

	// Final verification via the API: the new paragraph should now be in Notion.
	blocks, err := client.GetBlocks(context.Background(), newID)
	if err != nil {
		t.Fatalf("GetBlocks: %v", err)
	}
	var foundUpdate bool
	for _, b := range blocks {
		if pc, ok := b.Content.(*notion.ParagraphContent); ok {
			for _, rt := range pc.RichText {
				if strings.Contains(rt.PlainText, updateLine) {
					foundUpdate = true
				}
			}
		}
	}
	if !foundUpdate {
		t.Errorf("update line %q not found in Notion after second push", updateLine)
	}
}

// TestE2E_NewDB exercises the full nodin new-db CLI flow:
// init → pipe interactive answers to new-db → verify the database exists on
// Notion (via API) and the local _schema.json + index entry were written.
func TestE2E_NewDB(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()
	client := notion.NewClient(token, 3)

	initWorkspace(t, dir, token, rootPageID)

	title := fmt.Sprintf("nodin-e2e-db-%d", rand.Int63n(1_000_000_000))

	// Interactive answers, in the same order the prompts appear:
	//   Database title
	//   Parent (empty → defaults to RootPageID from config)
	//   Property 1: Name / title
	//   Property 2: Status / select / option Todo / color gray / option Done / color green / empty (finish options)
	//   Property 3: empty name (finish properties)
	stdin := strings.Join([]string{
		title,
		"",
		"Name",
		"title",
		"Status",
		"select",
		"Todo", "gray",
		"Done", "green",
		"",
		"",
	}, "\n") + "\n"

	cmd := exec.Command(nodinBin, "new-db")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	combinedOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("new-db failed: %v\noutput:\n%s", err, combinedOut)
	}
	out := string(combinedOut)
	if !strings.Contains(out, "created database") {
		t.Errorf("new-db output missing 'created database':\n%s", out)
	}

	// Locate the new database in the index so we can clean it up and verify
	// it on the Notion side.
	type indexEntry struct {
		NotionID  string `json:"notion_id"`
		LocalPath string `json:"local_path"`
		Type      string `json:"type"`
	}
	var idx map[string]indexEntry
	idxData, err := os.ReadFile(filepath.Join(dir, ".nodin", "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := json.Unmarshal(idxData, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	var dbID, dbLocalPath string
	for id, e := range idx {
		if e.Type == "database" {
			dbID = id
			dbLocalPath = e.LocalPath
			break
		}
	}
	if dbID == "" {
		t.Fatalf("no database entry in index; index:\n%s", idxData)
	}
	t.Cleanup(func() { _ = client.ArchiveDatabase(context.Background(), dbID) })

	// _schema.json was written under the canonical databases/<slug>-<shortID>/ path.
	schemaPath := filepath.Join(dir, dbLocalPath, "_schema.json")
	if _, err := os.Stat(schemaPath); err != nil {
		t.Errorf("_schema.json missing at %s: %v", schemaPath, err)
	}

	// Notion-side verification: title and property types match.
	db, err := client.GetDatabase(context.Background(), dbID)
	if err != nil {
		t.Fatalf("GetDatabase: %v", err)
	}
	if db.TitleText() != title {
		t.Errorf("notion title = %q, want %q", db.TitleText(), title)
	}
	schemaFromNotion := db.Schema() // omits "title"
	if schemaFromNotion["Status"] != "select" {
		t.Errorf("Status type on Notion = %q, want select", schemaFromNotion["Status"])
	}
}

// TestE2E_PushCreatesDatabase exercises the auto-create-on-push flow: the user
// drops a _schema.json under databases/<slug>/ and runs push. The DB must be
// created on Notion, the local path preserved, and the push summary must say
// "1 databases created".
func TestE2E_PushCreatesDatabase(t *testing.T) {
	token, rootPageID := e2eCredentials(t)
	dir := t.TempDir()
	client := notion.NewClient(token, 3)

	initWorkspace(t, dir, token, rootPageID)

	slug := fmt.Sprintf("nodin-e2e-pushdb-%d", rand.Int63n(1_000_000_000))
	dbDir := filepath.Join(dir, "databases", slug)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	schemaJSON := fmt.Sprintf(`{
		"title": %q,
		"properties": {
			"Name":  { "type": "title" },
			"Notes": { "type": "rich_text" }
		}
	}`, slug)
	if err := os.WriteFile(filepath.Join(dbDir, "_schema.json"), []byte(schemaJSON), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, code := runIn(t, dir, "push")
	if code != 0 {
		t.Fatalf("push exited %d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "1 databases created") {
		t.Errorf("push output missing '1 databases created':\n%s", out)
	}

	// Index has a database entry at the user-chosen path.
	type indexEntry struct {
		LocalPath string `json:"local_path"`
		Type      string `json:"type"`
	}
	var idx map[string]indexEntry
	idxData, err := os.ReadFile(filepath.Join(dir, ".nodin", "index.json"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if err := json.Unmarshal(idxData, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	relPath := "databases/" + slug
	var dbID string
	for id, e := range idx {
		if e.Type == "database" && e.LocalPath == relPath {
			dbID = id
			break
		}
	}
	if dbID == "" {
		t.Fatalf("no database entry at %s; index:\n%s", relPath, idxData)
	}
	t.Cleanup(func() { _ = client.ArchiveDatabase(context.Background(), dbID) })

	// Notion-side verification.
	db, err := client.GetDatabase(context.Background(), dbID)
	if err != nil {
		t.Fatalf("GetDatabase: %v", err)
	}
	if db.TitleText() != slug {
		t.Errorf("notion title = %q, want %q", db.TitleText(), slug)
	}
}
