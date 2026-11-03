package hint

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

func TestOf(t *testing.T) {
	if got := Of("ab12"); got != "[raw:ab12]" {
		t.Errorf("Of = %q", got)
	}
}

// Every form ever sent still marks optimized output (transcripts and older
// sessions hold the long ones); prose that merely says "raw" or "full:" does not.
func TestRe(t *testing.T) {
	tests := []struct {
		name, text string
		want       bool
	}{
		{"end", "x\n" + Of("ab12"), true},
		{"top", Of("ab12") + "\nx", true},
		{"long raw", "x\n[gascoigne: optimized 100→20 tokens · full: gascoigne raw abc]", true},
		{"long mcp", "x\n[optimized 10→5 tokens · full: retrieve id=id1]", true},
		{"plain", "raw: stop", false},
		{"bracketed prose", "[raw output: see the docs]", false},
		{"counts only", "[gascoigne: optimized 10→5 tokens]", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Re.MatchString(tt.text); got != tt.want {
				t.Fatalf("%q: got %v", tt.text, got)
			}
		})
	}
}

// The point of the short form: it costs fewer tokens than the one with counts.
func TestShorter(t *testing.T) {
	long := "[gascoigne: optimized 1042→974 tokens · full: gascoigne raw 69938658]"
	if a, b := tokens.Estimate(Of("69938658")), tokens.Estimate(long); a >= b {
		t.Fatalf("short %d tokens, long %d", a, b)
	}
}
