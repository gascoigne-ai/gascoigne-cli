package terse

import (
	"strings"
	"testing"
)

func TestPrune(t *testing.T) {
	tests := []struct {
		name, in, want string
		ratio          float64
	}{
		{"articles first", "Run the tests and fix a bug in the parser.\n", "Run tests and fix bug in parser.\n", 0.3},
		{"fillers and hedges", "It is really just basically a simple fix.\n", "It is simple fix.\n", 0.6},
		{"negations and modals stay", "You must not run the tests only if they never fail.\n", "You must not run tests only if they never fail.\n", 0.9},
		{"code, quotes and paths stay", "Use `the a an` or \"the end\" in the/a.go and a_b.\n", "Use `the a an` or \"the end\" in the/a.go and a_b.\n", 0.9},
		{"headings, tables, fences stay", "# The title\n| the | a |\n```\nthe code\n```\nthe text\n", "# The title\n| the | a |\n```\nthe code\n```\ntext\n", 1},
		{"capitalized names stay", "May is the month. The end.\n", "May is month. end.\n", 0.6},
		{"ratio 0 changes nothing", "the a an\n", "the a an\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Prune(tt.in, tt.ratio); got != tt.want {
				t.Fatalf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestPruneDeterministic(t *testing.T) {
	text := strings.Repeat("Keep the output of a run short and clear, so the agent reads it quickly.\n", 20)
	if Prune(text, 0.2) != Prune(text, 0.2) {
		t.Fatal("not deterministic")
	}
}
