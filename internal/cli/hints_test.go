package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirstConflictLine(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "no_markers",
			content: "line1\nline2\nline3\n",
			want:    0,
		},
		{
			name:    "marker_at_line_1",
			content: "<<<<<<< ours\nlocal\n=======\nremote\n>>>>>>> theirs\n",
			want:    1,
		},
		{
			name:    "marker_at_line_3",
			content: "line1\nline2\n<<<<<<< ours\nlocal\n=======\nremote\n>>>>>>> theirs\n",
			want:    3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "nodin-hints-test-*")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString(tt.content); err != nil {
				t.Fatal(err)
			}
			_ = f.Close()
			if got := firstConflictLine(f.Name()); got != tt.want {
				t.Errorf("firstConflictLine = %d, want %d", got, tt.want)
			}
		})
	}

	t.Run("missing_file", func(t *testing.T) {
		if got := firstConflictLine("/nonexistent/path/does/not/exist"); got != 0 {
			t.Errorf("expected 0 for missing file, got %d", got)
		}
	})
}

func TestPrintConflictHints_Empty(t *testing.T) {
	var buf bytes.Buffer
	printConflictHints(&buf, nil, func(p string) string { return p })
	if buf.Len() != 0 {
		t.Errorf("expected no output for nil paths, got %q", buf.String())
	}

	printConflictHints(&buf, []string{}, func(p string) string { return p })
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty paths, got %q", buf.String())
	}
}

func TestPrintConflictHints_WithEditor(t *testing.T) {
	t.Setenv("VISUAL", "nvim")
	t.Setenv("EDITOR", "")

	dir := t.TempDir()
	relPath := "pages/conflict.md"
	abs := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	// conflict marker is on line 2
	if err := os.WriteFile(abs, []byte("before\n<<<<<<< ours\nlocal\n=======\nremote\n>>>>>>> theirs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	printConflictHints(&buf, []string{relPath}, func(p string) string {
		return filepath.Join(dir, p)
	})

	out := buf.String()
	if !strings.Contains(out, "nvim +2 "+relPath) {
		t.Errorf("expected 'nvim +2 %s' in output, got:\n%s", relPath, out)
	}
	if !strings.Contains(out, "nodin push") {
		t.Errorf("expected 'nodin push' hint in output, got:\n%s", out)
	}
}

func TestPrintConflictHints_FallsBackToEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "vim")

	dir := t.TempDir()
	relPath := "doc.md"
	abs := filepath.Join(dir, relPath)
	if err := os.WriteFile(abs, []byte("<<<<<<< ours\na\n=======\nb\n>>>>>>> theirs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	printConflictHints(&buf, []string{relPath}, func(p string) string {
		return filepath.Join(dir, p)
	})

	out := buf.String()
	if !strings.Contains(out, "vim +1 "+relPath) {
		t.Errorf("expected 'vim +1 %s' in output, got:\n%s", relPath, out)
	}
}

func TestPrintConflictHints_NoEditor(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")

	dir := t.TempDir()
	relPath := "pages/note.md"
	abs := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		t.Fatal(err)
	}
	// conflict marker at line 3
	if err := os.WriteFile(abs, []byte("a\nb\n<<<<<<< ours\nc\n=======\nd\n>>>>>>> theirs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	printConflictHints(&buf, []string{relPath}, func(p string) string {
		return filepath.Join(dir, p)
	})

	out := buf.String()
	if !strings.Contains(out, relPath) {
		t.Errorf("expected path in output, got:\n%s", out)
	}
	if !strings.Contains(out, "line 3") {
		t.Errorf("expected 'line 3' in output, got:\n%s", out)
	}
}

func TestPrintConflictHints_MultipleFiles(t *testing.T) {
	t.Setenv("VISUAL", "hx")
	t.Setenv("EDITOR", "")

	dir := t.TempDir()
	paths := []string{"a.md", "b.md"}
	for _, p := range paths {
		if err := os.WriteFile(filepath.Join(dir, p), []byte("<<<<<<< ours\nx\n=======\ny\n>>>>>>> theirs\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var buf bytes.Buffer
	printConflictHints(&buf, paths, func(p string) string {
		return filepath.Join(dir, p)
	})

	out := buf.String()
	for _, p := range paths {
		if !strings.Contains(out, p) {
			t.Errorf("expected %q in output, got:\n%s", p, out)
		}
	}
}
