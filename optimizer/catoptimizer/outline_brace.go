package catoptimizer

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	container = lazyre.New(`\b(class|interface|enum|struct|union|namespace|module|mod|package|impl|trait|object|record)\b`)
	// extern "C" { and extern(C) { hold declarations, not a body; extern "C" fn does.
	externBlock = lazyre.New(`\bextern\s*("[^"]*"|\(\w+(\+\+)?\))?\s*\{`)
)

// containerEnd is the index where the first container keyword of l ends (a
// brace after it opens a container block), -1 when there is none.
func containerEnd(line string) int {
	if m := container.FindStringIndex(line); m != nil {
		return m[1]
	}
	if m := externBlock.FindStringIndex(line); m != nil {
		return m[1] - 1
	}
	return -1
}

// braceOutline elides bodies of blocks nested only in containers (classes,
// namespaces): functions and methods, top-level object literals. Strings and
// comments are skipped roughly; good enough for an outline.
func (o *Optimizer) braceOutline(lines []string, lang detect.Language) []span {
	var stack []scope
	elide := map[int]int{} // first body line → last body line
	sc := lexer{lang: lang}
	for i, l := range lines {
		// A block opens in a container when a keyword precedes its brace:
		// the line's first keyword ends before it. Found once per line, not
		// per brace, so minified one-line sources stay linear.
		kw := -1
		if strings.IndexByte(l, '{') >= 0 {
			kw = containerEnd(l)
		}
		for j := 0; j < len(l); j++ {
			if end, skipped := sc.skip(l, j); skipped {
				j = end
				continue
			}
			switch l[j] {
			case '{':
				stack = append(stack, scope{i, kw >= 0 && kw <= j})
			case '}':
				stack = o.closeBlock(stack, i, elide)
			}
		}
	}
	return spans(elide)
}

// scope is an open block and whether it holds declarations. at is the line a
// brace block opened on, or the indentation of an indent-based declaration.
type scope struct {
	at        int
	container bool
}

// onlyContainers reports whether every open block is a container.
func onlyContainers(stack []scope) bool {
	return !slices.ContainsFunc(stack, func(frame scope) bool { return !frame.container })
}

// closeBlock closes the innermost block at line, eliding its body when it is
// a long function nested only in containers.
func (o *Optimizer) closeBlock(stack []scope, line int, elide map[int]int) []scope {
	if len(stack) == 0 {
		return stack
	}
	top := stack[len(stack)-1]
	stack = stack[:len(stack)-1]
	if !top.container && onlyContainers(stack) && line-top.at-1 >= o.cfg.MinBody {
		elide[top.at+1] = line - 1
	}
	return stack
}

// lexer tells code from comments and strings across the lines of a source.
type lexer struct {
	lang    detect.Language
	closers []string // of the block comments we are in, innermost last
	raw     string   // closer of the raw string we are in
}

// skip reports whether l[j] starts or continues a comment or string, and
// the index of the last byte of it on this line (len(l) when it goes on).
func (x *lexer) skip(line string, pos int) (end int, skipped bool) {
	switch {
	case x.raw != "":
		if strings.HasPrefix(line[pos:], x.raw) {
			pos += len(x.raw) - 1
			x.raw = ""
		}
		return pos, true
	case len(x.closers) > 0:
		return x.inComment(line, pos), true
	case strings.HasPrefix(line[pos:], cmp.Or(x.lang.Line, "//")):
		return len(line), true
	}
	if closer := x.commentOpener(line[pos:]); closer != "" {
		x.closers = append(x.closers, closer)
		return pos + 1, true
	}
	switch c := line[pos]; {
	case c == '\'':
		return x.quote(line, pos)
	case c == '"' || c == '`':
		return skipString(line, pos), true
	case c == 'r' && x.lang.Raw == detect.RawRust, c == 'R' && x.lang.Raw == detect.RawCpp:
		return x.rawStart(line, pos)
	}
	return pos, false
}

// inComment moves through a block comment, opening nested ones.
func (x *lexer) inComment(line string, pos int) int {
	if rest := line[pos:]; strings.HasPrefix(rest, x.closers[len(x.closers)-1]) {
		pos += len(x.closers[len(x.closers)-1]) - 1
		x.closers = x.closers[:len(x.closers)-1]
	} else if closer := x.commentOpener(rest); closer != "" && x.nests(closer) {
		x.closers = append(x.closers, closer)
		pos++
	}
	return pos
}

// commentOpener is the closer of the block comment l starts with, "" when it
// starts with none. Both delimiters are two bytes, so callers step one past.
func (x *lexer) commentOpener(line string) string {
	switch {
	case strings.HasPrefix(line, "/*"):
		return "*/"
	case x.lang.Nested[0] != "" && strings.HasPrefix(line, x.lang.Nested[0]):
		return x.lang.Nested[1]
	}
	return ""
}

// nests reports whether a comment closed by closer may hold another.
func (x *lexer) nests(closer string) bool {
	return closer == x.lang.Nested[1] || x.lang.NestedBlock
}

// quote handles ': a char literal, a lifetime, a digit separator or a string.
func (x *lexer) quote(line string, pos int) (int, bool) {
	switch {
	case x.lang.DigitSep && pos > 0 && compress.IsDigit(line[pos-1]) && pos+1 < len(line) && compress.IsDigit(line[pos+1]):
		return pos, true
	case x.lang.Lifetimes:
		if pos+1 < len(line) && line[pos+1] != '\\' {
			_, w := utf8.DecodeRuneInString(line[pos+1:])
			if pos+1+w >= len(line) || line[pos+1+w] != '\'' {
				return pos, true // 'a: no closing quote follows
			}
		}
	}
	return skipString(line, pos), true
}

func isIdent(char byte) bool {
	return char == '_' || compress.IsDigit(char) || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
}

// rawStart opens a raw string at l[j] (r#"…"# in Rust, R"x(…)x" in C++);
// anything else is an identifier and not skipped.
func (x *lexer) rawStart(line string, pos int) (int, bool) {
	if pos > 0 && isIdent(line[pos-1]) && !strings.ContainsRune("bLuU8", rune(line[pos-1])) {
		return pos, false
	}
	rest := line[pos+1:]
	if x.lang.Raw == detect.RawRust {
		hashes := len(rest) - len(strings.TrimLeft(rest, "#"))
		if !strings.HasPrefix(rest[hashes:], `"`) {
			return pos, false
		}
		x.raw = `"` + rest[:hashes]
		return pos + hashes + 1, true
	}
	delim, _, ok := strings.Cut(strings.TrimPrefix(rest, `"`), "(")
	if !strings.HasPrefix(rest, `"`) || !ok || len(delim) > 16 || strings.ContainsAny(delim, ` )\\"`) {
		return pos, false
	}
	x.raw = ")" + delim + `"`
	return pos + 2 + len(delim), true
}

func skipString(line string, pos int) int {
	q := line[pos]
	for k := pos + 1; k < len(line); k++ {
		switch line[k] {
		case '\\':
			k++
		case q:
			return k
		}
	}
	return len(line) // unterminated on this line (template literal): ignore rest
}
