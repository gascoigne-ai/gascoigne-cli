package pcomb

import (
	"regexp"
	"strings"
	"testing"
)

func TestMatchers(t *testing.T) {
	tests := []struct {
		name string
		m    Matcher
		yes  []string
		no   []string
	}{
		{"prefix", Prefix("ab", "x"),
			[]string{"ab", "abc", "x", "xyz", "ab\r"},
			[]string{"", "a", "b", " ab", "ba", "yx", "AB"}},
		{"prefix overlapping terms", Prefix("a", "ab"),
			[]string{"a", "ab", "abc"},
			[]string{"b"}},
		{"prefix unicode", Prefix("✓ "),
			[]string{"✓ 12 modules", "✓ "},
			[]string{"✓", "x✓ "}},
		{"word", Word("Compiling", "Checking"),
			[]string{"Compiling", "Compiling x", "Checking\tfoo", "Checking:", "Compiling\r"},
			[]string{"", "Compilingly", "Compiling_x", "Compiling9", " Compiling", "compiling", "Compil"}},
		{"word unicode neighbour is a boundary like regexp", Word("go"),
			[]string{"go é"},
			[]string{"goal"}},
		{"word longer keyword listed after shorter", Word("a", "ab"),
			[]string{"a b", "ab c", "ab"},
			[]string{"abc"}},
		{"exact", Exact("Offenses:", "Done"),
			[]string{"Offenses:", "Done"},
			[]string{"", "Offenses: 3", "Done ", " Done", "done"}},
		{"prefixed", Prefixed("[INFO] ", Prefix("Download", "---")),
			[]string{"[INFO] Downloading x", "[INFO] --- plugin", "[INFO] Download"},
			[]string{"", "[INFO]", "[INFO] ", "[INFO]  Download", "[WARN] Download", "Download"}},
		{"prefixed with whole-line inner matcher", Prefixed("[INFO] ", Exact("")),
			[]string{"[INFO] "},
			[]string{"[INFO]", "[INFO]  "}},
		{"indented", Indented(Word("Finished")),
			[]string{" Finished", "\tFinished in 2s", "    Finished", "\n Finished"},
			[]string{"Finished", "", "   ", " Finishedx", " x Finished"}},
		{"trimmed", Trimmed(Prefix("Inspecting")),
			[]string{"Inspecting", " Inspecting 3 files", "\t\tInspecting"},
			[]string{"", "  ", "xInspecting"}},
		{"any", Any(Prefix("a"), Word("b")),
			[]string{"a1", "b", "b c"},
			[]string{"", "bc", "c"}},
		{"any empty never matches", Any(),
			nil,
			[]string{"", "x"}},
		{"re anchored", Re(`^\d+ warnings?$`),
			[]string{"1 warning", "20 warnings"},
			[]string{"", "warnings", "x 1 warning", "1 warning x"}},
		{"re unanchored matches anywhere", Re(`node_modules`),
			[]string{"at x (node_modules/y)", "node_modules"},
			[]string{"", "node_module"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range tt.yes {
				if !tt.m(s) {
					t.Errorf("should match %q", s)
				}
			}
			for _, s := range tt.no {
				if tt.m(s) {
					t.Errorf("should not match %q", s)
				}
			}
		})
	}
}

func TestConstructorsRejectNonsense(t *testing.T) {
	tests := []struct {
		name string
		make func()
	}{
		{"Prefix without args", func() { Prefix() }},
		{"Prefix with empty string", func() { Prefix("a", "") }},
		{"Word without args", func() { Word() }},
		{"Word with empty string", func() { Word("") }},
		{"Exact without args", func() { Exact() }},
		{"Prefixed with empty prefix", func() { Prefixed("", Prefix("a")) }},
		{"Word ending in punctuation", func() { Word("Compiling:") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("should panic")
				}
			}()
			tt.make()
		})
	}
}

// Every matcher must agree with the regular expression it replaces; the
// strings mix keyword edges, whitespace and non-ASCII on purpose.
var diffLines = []string{
	"", " ", "\t", "\r\n", "Compiling", "Compiling foo", " Compiling foo", "\tCompiling", "Compilingly",
	"Compiling_x", "Compiling9", "Compiling\r", "Compiling é", "Checking", "Finished in 1s", "  Finished",
	"✓ 3 modules transformed", "✓", "ab", "abc", "a", "b", "x", "_", "é", "\u00a0Compiling", "\f\vCompiling",
}

func TestAgreesWithRegexp(t *testing.T) {
	tests := []struct {
		name string
		m    Matcher
		re   string
	}{
		{"prefix", Prefix("ab", "Compiling", "✓ "), `^(ab|Compiling|✓ )`},
		{"prefix with metacharacters", Prefix("a.b", "(x"), `^(a\.b|\(x)`},
		{"word", Word("Compiling", "Checking", "a"), `^(Compiling|Checking|a)\b`},
		{"indented word", Indented(Word("Compiling", "Finished")), `^\s+(Compiling|Finished)\b`},
		{"trimmed prefix", Trimmed(Prefix("Compiling", "ab")), `^\s*(Compiling|ab)`},
		{"any", Any(Prefix("ab"), Indented(Word("Finished"))), `^ab|^\s+Finished\b`},
		{"exact", Exact("ab", "Compiling"), `^(ab|Compiling)$`},
		{"prefixed", Prefixed("ab", Word("Compiling", "a")), `^ab(Compiling|a)\b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.re)
			for _, s := range diffLines {
				if got, want := tt.m(s), re.MatchString(s); got != want {
					t.Errorf("%q: got %v, regexp %v", s, got, want)
				}
			}
		})
	}
}

func FuzzAgreesWithRegexp(f *testing.F) {
	for _, s := range diffLines {
		f.Add(s)
	}
	word := Word("Compiling", "Checking", "a")
	indented := Indented(Word("Compiling", "Finished"))
	prefix := Prefix("ab", "Compiling", "✓ ")
	wordRe := regexp.MustCompile(`^(Compiling|Checking|a)\b`)
	indentedRe := regexp.MustCompile(`^\s+(Compiling|Finished)\b`)
	prefixRe := regexp.MustCompile(`^(ab|Compiling|✓ )`)
	f.Fuzz(func(t *testing.T, s string) {
		// regexp reads invalid UTF-8 as U+FFFD, which is never a word character
		// either, so the two still agree on arbitrary bytes.
		if got, want := word(s), wordRe.MatchString(s); got != want {
			t.Errorf("Word %q: got %v, regexp %v", s, got, want)
		}
		if got, want := indented(s), indentedRe.MatchString(s); got != want {
			t.Errorf("Indented %q: got %v, regexp %v", s, got, want)
		}
		if got, want := prefix(s), prefixRe.MatchString(s); got != want {
			t.Errorf("Prefix %q: got %v, regexp %v", s, got, want)
		}
	})
}

func TestLongLine(t *testing.T) {
	long := strings.Repeat(" ", 1<<20) + "Compiling x"
	if !Indented(Word("Compiling"))(long) {
		t.Error("Indented should match after a megabyte of spaces")
	}
	if Prefix("Compiling")(strings.Repeat("x", 1<<20)) {
		t.Error("Prefix should not match a long unrelated line")
	}
}

func TestMatchersDoNotAllocate(t *testing.T) {
	m := Any(Prefix("ab"), Indented(Word("Compiling", "Finished")))
	line := "   Finished in 2s"
	if n := testing.AllocsPerRun(100, func() { m(line) }); n != 0 {
		t.Errorf("allocs per match = %v, want 0", n)
	}
}

func TestReIsLazy(t *testing.T) {
	// A broken pattern must not panic at construction: lazyre.Check reports it
	// in tests and the first match would fail loudly.
	defer func() {
		if recover() != nil {
			t.Error("Re compiled its pattern at construction")
		}
	}()
	Re(`(`)
}
