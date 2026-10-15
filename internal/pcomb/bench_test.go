package pcomb

import (
	"regexp"
	"testing"
)

var benchLines = []string{"   Compiling foo v0.1.0", "error[E0308]: mismatched types", "    Finished dev profile", "  --> src/main.rs:4:5"}

func BenchmarkIndentedWord(b *testing.B) {
	m := Indented(Word("Compiling", "Checking", "Downloaded", "Downloading", "Updating", "Locking", "Blocking", "Fresh", "Finished"))
	for b.Loop() {
		for _, l := range benchLines {
			m(l)
		}
	}
}

func BenchmarkRegexpEquivalent(b *testing.B) {
	re := regexp.MustCompile(`^\s+(Compiling|Checking|Downloaded|Downloading|Updating|Locking|Blocking|Fresh|Finished)\b`)
	for b.Loop() {
		for _, l := range benchLines {
			re.MatchString(l)
		}
	}
}
