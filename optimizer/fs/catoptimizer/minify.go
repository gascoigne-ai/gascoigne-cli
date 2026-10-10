package catoptimizer

import (
	"strings"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/lazyre"
)

// syntax is what minifying needs to know about a language: its comments and
// the multi-line strings whose lines must never be taken for comments.
type syntax struct {
	line       string    // line comment token
	block      [2]string // block comment delimiters; empty when none
	strs       []string  // delimiters of strings that can span lines
	heredoc    bool      // <<EOF … EOF (shell, ruby, perl, php)
	docstrings bool      // python: a string right after def/class is documentation
}

var (
	slash  = syntax{line: "//", block: [2]string{"/*", "*/"}, strs: []string{"`", `"""`}}
	hash   = syntax{line: "#", strs: []string{`"""`, `'''`}, heredoc: true}
	python = syntax{line: "#", strs: []string{`"""`, `'''`}, docstrings: true}
	dash   = syntax{line: "--"}
	semi   = syntax{line: ";"}
)

var syntaxByExt = map[string]syntax{
	".rb": hash, ".pl": hash, ".pm": hash, ".r": hash, ".jl": hash, ".nim": hash, ".ex": hash,
	".exs": hash, ".tf": hash, ".ps1": hash,
	".lua": dash, ".hs": dash, ".elm": dash,
	".clj": semi, ".cljs": semi, ".el": semi, ".lisp": semi, ".scm": semi,
	".erl": {line: "%"}, ".hrl": {line: "%"},
	".php": {line: "//", block: slash.block, strs: slash.strs, heredoc: true},
}

// syntaxOf picks comment syntax by file extension, else by language family;
// ok is false for languages whose comments are not known.
func syntaxOf(lang string, h detect.Hints) (syntax, bool) {
	if s, ok := syntaxByExt[detect.SourceExt(h)]; ok {
		return s, true
	}
	switch lang {
	case detect.LangGo, detect.LangCLike:
		return slash, true
	case detect.LangPython:
		return python, true
	case detect.LangShell:
		return hash, true
	}
	return syntax{}, false
}

// directive matches comments that tools read: build tags, linters, type
// checkers, encodings, shebangs. They stay even when other comments go.
var directive = lazyre.New(`^(#!|//go:|//\s*\+build|//\s*(nolint|NOLINT|eslint|prettier|biome|istanbul|c8|@ts-|#region|#endregion|swiftlint|clang-format)|///\s*<reference|#\s*(type:|noqa|pragma|pylint|mypy|pyright|fmt:|-\*-|frozen_string_literal|encoding|rubocop|shellcheck)|--\s*(luacheck|selene))`)

// span is a range of original lines [from, to] replaced by one marker.
type span struct {
	from, to int
	what     string // "comments", "docstring" or "" for code bodies
	indent   string // of the marker, for comment spans
}

// comments finds comment runs that can go. Lines are only ever dropped
// whole: what stays is byte-exact, so an agent can copy it into an edit.
// A run right above code documents it: its first line with text stays.
func comments(lines []string, syn syntax) []span {
	var spans []span
	open := ""     // delimiter of the multi-line string we are in
	lastCode := -1 // index of the last code line
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		switch {
		case open != "":
			if closes(l, open) {
				open = ""
			}
			continue
		case t == "":
			continue
		case syn.docstrings && isDocstring(t) && (lastCode < 0 || strings.HasSuffix(code(lines[lastCode], syn), ":")):
			// The summary line stays, so a body is never left empty.
			end := docstringEnd(lines, i, t)
			if first := firstText(lines, i, end); first < end {
				spans = appendRun(spans, lines, i, first+1, end, "docstring")
			}
			i, lastCode = end, end
			continue
		case isComment(t, syn) && commentEnd(lines, i, syn) >= i:
			end := commentEnd(lines, i, syn)
			doc := end+1 < len(lines) && strings.TrimSpace(lines[end+1]) != ""
			if !doc {
				spans = appendRun(spans, lines, i, i, end, "comments")
			} else if first := firstText(lines, i, end); first < end {
				spans = appendRun(spans, lines, i, first+1, end, "comments")
			}
			i = end
			continue
		}
		lastCode = i
		open = opens(code(l, syn), syn)
	}
	return spans
}

// appendRun adds lines [from, to] of the run starting at line start, unless
// a marker would cost about as much or the run holds a directive. The marker
// takes the start's indent: continuation lines (" * …", docstring text) may
// sit elsewhere.
func appendRun(spans []span, lines []string, start, from, to int, what string) []span {
	size := 0
	for _, l := range lines[from : to+1] {
		if directive.MatchString(strings.TrimSpace(l)) {
			return spans
		}
		size += len(strings.TrimSpace(l))
	}
	if size < 2*len("// … comments L100-111") {
		return spans
	}
	return append(spans, span{from: from, to: to, what: what, indent: lines[start][:indentOf(lines[start])]})
}

func isComment(t string, syn syntax) bool {
	if directive.MatchString(t) {
		return false
	}
	return syn.line != "" && strings.HasPrefix(t, syn.line) ||
		syn.block[0] != "" && strings.HasPrefix(t, syn.block[0])
}

// commentEnd is the last line of the comment run starting at i, i-1 when
// there is none. A block comment counts only when nothing but space follows
// its closing delimiter.
func commentEnd(lines []string, i int, syn syntax) int {
	end := i - 1
	for j := i; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		switch {
		case syn.line != "" && strings.HasPrefix(t, syn.line) && !directive.MatchString(t):
			end = j
		case syn.block[0] != "" && strings.HasPrefix(t, syn.block[0]):
			k := blockEnd(lines, j, syn.block[1])
			if k < 0 {
				return end
			}
			end, j = k, k
		default:
			return end
		}
	}
	return end
}

// blockEnd is the line closing a block comment opened on line i, -1 when
// code follows the delimiter or it never closes.
func blockEnd(lines []string, i int, close string) int {
	first := strings.TrimSpace(lines[i])[2:]
	for j := i; j < len(lines); j++ {
		l := lines[j]
		if j == i {
			l = first
		}
		if k := strings.Index(l, close); k >= 0 {
			if strings.TrimSpace(l[k+len(close):]) != "" {
				return -1
			}
			return j
		}
	}
	return -1
}

var docOpen = lazyre.New(`^[rRuUbB]{0,2}("""|''')`)

func isDocstring(t string) bool { return docOpen.MatchString(t) }

// docstringEnd is the line closing the docstring opened on line i.
func docstringEnd(lines []string, i int, t string) int {
	q := docOpen.FindStringSubmatch(t)[1]
	rest := t[strings.Index(t, q)+3:]
	if strings.Contains(rest, q) {
		return i
	}
	for j := i + 1; j < len(lines); j++ {
		if strings.Contains(lines[j], q) {
			return j
		}
	}
	return len(lines) - 1
}

// firstText is the first line of a run with words in it ("/**" has none).
func firstText(lines []string, from, to int) int {
	for i := from; i <= to; i++ {
		if strings.IndexFunc(lines[i], isLetter) >= 0 {
			return i
		}
	}
	return from
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// code is l without a trailing line comment, roughly: a token inside a
// string cuts too early, which only makes string tracking more cautious.
func code(l string, syn syntax) string {
	if syn.line == "" {
		return strings.TrimSpace(l)
	}
	if k := strings.Index(l, " "+syn.line); k >= 0 {
		l = l[:k]
	}
	return strings.TrimSpace(l)
}

var heredoc = lazyre.New(`<<[-~]?\s*['"]?(\w+)['"]?`)

// opens returns the delimiter of a multi-line string left open by line l:
// an odd count of a string delimiter, or a heredoc whose terminator follows.
func opens(l string, syn syntax) string {
	for _, d := range syn.strs {
		if strings.Count(l, d)%2 == 1 {
			return d
		}
	}
	if syn.heredoc {
		if m := heredoc.FindStringSubmatch(l); m != nil {
			return "\n" + m[1] // heredocs close on a line equal to the terminator
		}
	}
	return ""
}

func closes(l, open string) bool {
	if term, ok := strings.CutPrefix(open, "\n"); ok {
		return strings.TrimSpace(l) == term
	}
	return strings.Count(l, open)%2 == 1
}
