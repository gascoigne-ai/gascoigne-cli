package diag

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	// "t.cc:10:12:   required from here", "In instantiation of 'X<int>':" (gcc, clang notes)
	instantiationRe = lazyre.New(`^\S+:\d+:\d+:\s+(recursively )?required (from|by)\b|^\S+: In (instantiation|member function|function|constructor|destructor|static member function|lambda function|substitution)\b|^\s+(recursively )?required from\b`)
	// "In file included from a.h:3:" and its "                 from b.h:5:" continuations
	includeRe = lazyre.New(`^In file included from \S+:\d+[:,]|^\s+from \S+\.(h|hh|hpp|hxx|c|cc|cpp|cxx|inl|ipp|tpp):\d+[:,]`)
	// "/usr/bin/ld: a.o: in function `main':" and "a.cc:(.text+0x12): undefined reference to `X::f()'"
	linkContextRe = lazyre.New(`^(\S*ld(\.\w+)?: )?\S+: in function .*:$|^\S*ld(\.\w+)?: \S+\.o:`)
	undefinedRe   = lazyre.New(`undefined reference to [\x60'](.+)'$`)
)

// collapse shortens what compilers print around a diagnostic: runs of
// template instantiation notes and include chains keep their ends and
// count the middle, and a linker's undefined references become one list.
func collapse(lines []string, keepEnds int) []string {
	lines = collapseRuns(lines, instantiationRe, keepEnds, "template instantiation notes")
	lines = collapseRuns(lines, includeRe, keepEnds, "include chain lines")
	return foldUndefined(lines)
}

// collapseRuns keeps the first keepEnds and the last line of every run of
// lines matching re longer than keepEnds+2.
func collapseRuns(lines []string, re *lazyre.Regexp, keepEnds int, what string) []string {
	var out []string
	for i := 0; i < len(lines); {
		if !re.MatchString(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && re.MatchString(lines[j]) {
			j++
		}
		if n := j - i; n > keepEnds+2 {
			out = append(out, lines[i:i+keepEnds]...)
			out = append(out, compress.Omitted(n-keepEnds-1, what))
			out = append(out, lines[j-1])
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	return out
}

// foldUndefined replaces linker "undefined reference" lines and the
// function context lines between them by one list at the first of them.
func foldUndefined(lines []string) []string {
	var syms []string
	for _, l := range lines {
		if m := undefinedRe.FindStringSubmatch(l); m != nil && !slices.Contains(syms, m[1]) {
			syms = append(syms, m[1])
		}
	}
	if len(syms) < 3 {
		return lines
	}
	var out []string
	listed := false
	for _, l := range lines {
		if undefinedRe.MatchString(l) || linkContextRe.MatchString(l) {
			if !listed {
				out = append(out, fmt.Sprintf("undefined reference (%d symbols): %s", len(syms), strings.Join(syms, ", ")))
				listed = true
			}
			continue
		}
		out = append(out, l)
	}
	return out
}
