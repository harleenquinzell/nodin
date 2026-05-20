package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// printConflictHints writes a hint block for a list of conflicted file paths
// (relative to syncDir). absPath(rel) must convert a relative path to absolute.
func printConflictHints(w io.Writer, paths []string, absPath func(string) string) {
	if len(paths) == 0 {
		return
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	_, _ = fmt.Fprintln(w, "\nresolve conflicts:")
	for _, p := range paths {
		line := firstConflictLine(absPath(p))
		if editor == "" {
			if line > 0 {
				_, _ = fmt.Fprintf(w, "  %s  (line %d)\n", p, line)
			} else {
				_, _ = fmt.Fprintf(w, "  %s\n", p)
			}
		} else if line > 0 {
			_, _ = fmt.Fprintf(w, "  %s +%d %s\n", editor, line, p)
		} else {
			_, _ = fmt.Fprintf(w, "  %s %s\n", editor, p)
		}
	}
	_, _ = fmt.Fprintln(w, "then run: nodin push")
}

// firstConflictLine returns the 1-based line number of the first <<<<<<< marker,
// or 0 if none is found.
func firstConflictLine(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	s := bufio.NewScanner(f)
	n := 0
	for s.Scan() {
		n++
		if strings.HasPrefix(s.Text(), "<<<<<<<") {
			return n
		}
	}
	return 0
}
