package taskoptimizer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// corpus is every distinct line of the eval fixtures: real tool output, the
// characterization input for converted noise matchers.
func corpus(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	var lines []string
	err := filepath.WalkDir("../../testdata", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, l := range strings.Split(string(data), "\n") {
			if !seen[l] {
				seen[l] = true
				lines = append(lines, l)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

// sameAsOld pins a converted matcher to the regexp it replaced on the
// fixture lines and on edits of them and of the rule examples.
func sameAsOld(t *testing.T, old, now func(string) bool, examples []string) {
	t.Helper()
	hits := 0
	for _, l := range append(corpus(t), examples...) {
		for _, v := range []string{l, " " + l, "\t" + l, strings.TrimSpace(l), l + "x", l + " ", l[:len(l)/2], l[:max(len(l)-1, 0)]} {
			if old(v) != now(v) {
				t.Fatalf("old regexp says %v, new matcher says %v for %q", old(v), now(v), v)
			}
			if old(v) {
				hits++
			}
		}
	}
	if hits == 0 {
		t.Fatal("no line matched: the test pins nothing")
	}
}
