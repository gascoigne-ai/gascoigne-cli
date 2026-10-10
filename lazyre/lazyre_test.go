package lazyre

import (
	"strings"
	"testing"
)

func TestCompilesOnFirstUse(t *testing.T) {
	r := New(`a(\d+)`)
	if got := r.FindStringSubmatch("xa12"); len(got) != 2 || got[1] != "12" {
		t.Fatalf("got %q", got)
	}
	if r.String() != `a(\d+)` || r.Regexp() != r.Regexp() {
		t.Fatal("compiled twice or lost the pattern")
	}
}

func TestCheckReportsBadPatterns(t *testing.T) {
	New(`(unclosed`) // never used: only Check notices
	err := Check()
	if err == nil || !strings.Contains(err.Error(), "(unclosed") {
		t.Fatalf("Check() = %v", err)
	}
	all = all[:len(all)-1]
	if err := Check(); err != nil {
		t.Fatal(err)
	}
}
