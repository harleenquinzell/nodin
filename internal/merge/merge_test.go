package merge_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/harleenquinzell/nodin/internal/merge"
)

func hasGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func TestThreeWay_CleanMerge(t *testing.T) {
	if !hasGit() {
		t.Skip("git not available")
	}

	base := "line1\nline2\nline3\n"
	local := "line1\nline2 edited locally\nline3\n"
	remote := "line1\nline2\nline3\nline4 added remotely\n"

	res, err := merge.ThreeWay(base, local, remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Conflicts {
		t.Fatalf("expected no conflicts, got:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "line2 edited locally") {
		t.Errorf("missing local edit in merged output:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "line4 added remotely") {
		t.Errorf("missing remote addition in merged output:\n%s", res.Content)
	}
}

func TestThreeWay_Conflict(t *testing.T) {
	if !hasGit() {
		t.Skip("git not available")
	}

	base := "line1\nshared line\nline3\n"
	local := "line1\nlocal version\nline3\n"
	remote := "line1\nremote version\nline3\n"

	res, err := merge.ThreeWay(base, local, remote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Conflicts {
		t.Fatalf("expected conflicts, got clean merge:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "<<<<<<") {
		t.Errorf("expected conflict markers in output:\n%s", res.Content)
	}
}

func TestThreeWay_IdenticalSides(t *testing.T) {
	if !hasGit() {
		t.Skip("git not available")
	}

	base := "unchanged\n"
	res, err := merge.ThreeWay(base, base, base)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Conflicts {
		t.Fatalf("expected no conflicts for identical inputs")
	}
	if res.Content != base {
		t.Errorf("expected %q, got %q", base, res.Content)
	}
}
