package catoptimizer

import (
	"cmp"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// goOutline elides bodies of functions and methods using the Go parser.
// Signature and closing brace lines stay as they are.
func (o *Optimizer) goOutline(src string, lines []string) []span {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return o.braceOutline(lines, detect.Language{})
	}
	var spans []span
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		open, close := fset.Position(fn.Body.Lbrace).Line, fset.Position(fn.Body.Rbrace).Line
		ownLines := strings.HasSuffix(strings.TrimSpace(lines[open-1]), "{") && strings.TrimSpace(lines[close-1]) == "}"
		if ownLines && close-open-1 >= o.cfg.MinBody {
			spans = append(spans, span{from: open, to: close - 2})
		}
	}
	return spans
}

var (
	container = lazyre.New(`\b(class|interface|enum|struct|union|namespace|module|mod|package|impl|trait|object|record)\b`)
	// extern "C" { and extern(C) { hold declarations, not a body; extern "C" fn does.
	externBlock = lazyre.New(`\bextern\s*("[^"]*"|\(\w+(\+\+)?\))?\s*\{`)
)

// containerEnd is the index where the first container keyword of l ends (a
// brace after it opens a container block), -1 when there is none.
func containerEnd(l string) int {
	if m := container.FindStringIndex(l); m != nil {
		return m[1]
	}
	if m := externBlock.FindStringIndex(l); m != nil {
		return m[1] - 1
	}
	return -1
}

// braceOutline elides bodies of blocks nested only in containers (classes,
// namespaces): functions and methods, top-level object literals. Strings and
// comments are skipped roughly; good enough for an outline.
func (o *Optimizer) braceOutline(lines []string, lang detect.Language) []span {
	type open struct {
		line      int
		container bool
	}
	var stack []open
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
				stack = append(stack, open{i, kw >= 0 && kw <= j})
			case '}':
				if len(stack) == 0 {
					continue
				}
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				outer := !slices.ContainsFunc(stack, func(s open) bool { return !s.container })
				if !top.container && outer && i-top.line-1 >= o.cfg.MinBody {
					elide[top.line+1] = i - 1
				}
			}
		}
	}
	return spans(elide)
}

// lexer tells code from comments and strings across the lines of a source.
type lexer struct {
	lang    detect.Language
	closers []string // of the block comments we are in, innermost last
	raw     string   // closer of the raw string we are in
}

// skip reports whether l[j] starts or continues a comment or string, and
// the index of the last byte of it on this line (len(l) when it goes on).
func (x *lexer) skip(l string, j int) (end int, skipped bool) {
	switch {
	case x.raw != "":
		if strings.HasPrefix(l[j:], x.raw) {
			j += len(x.raw) - 1
			x.raw = ""
		}
		return j, true
	case len(x.closers) > 0:
		return x.inComment(l, j), true
	case strings.HasPrefix(l[j:], cmp.Or(x.lang.Line, "//")):
		return len(l), true
	}
	if closer := x.commentOpener(l[j:]); closer != "" {
		x.closers = append(x.closers, closer)
		return j + 1, true
	}
	switch c := l[j]; {
	case c == '\'':
		return x.quote(l, j)
	case c == '"' || c == '`':
		return skipString(l, j), true
	case c == 'r' && x.lang.Raw == detect.RawRust, c == 'R' && x.lang.Raw == detect.RawCpp:
		return x.rawStart(l, j)
	}
	return j, false
}

// inComment moves through a block comment, opening nested ones.
func (x *lexer) inComment(l string, j int) int {
	if rest := l[j:]; strings.HasPrefix(rest, x.closers[len(x.closers)-1]) {
		j += len(x.closers[len(x.closers)-1]) - 1
		x.closers = x.closers[:len(x.closers)-1]
	} else if closer := x.commentOpener(rest); closer != "" && x.nests(closer) {
		x.closers = append(x.closers, closer)
		j++
	}
	return j
}

// commentOpener is the closer of the block comment l starts with, "" when it
// starts with none. Both delimiters are two bytes, so callers step one past.
func (x *lexer) commentOpener(l string) string {
	switch {
	case strings.HasPrefix(l, "/*"):
		return "*/"
	case x.lang.Nested[0] != "" && strings.HasPrefix(l, x.lang.Nested[0]):
		return x.lang.Nested[1]
	}
	return ""
}

// nests reports whether a comment closed by closer may hold another.
func (x *lexer) nests(closer string) bool {
	return closer == x.lang.Nested[1] || x.lang.NestedBlock
}

// quote handles ': a char literal, a lifetime, a digit separator or a string.
func (x *lexer) quote(l string, j int) (int, bool) {
	switch {
	case x.lang.DigitSep && j > 0 && isDigit(l[j-1]) && j+1 < len(l) && isDigit(l[j+1]):
		return j, true
	case x.lang.Lifetimes:
		if j+1 < len(l) && l[j+1] != '\\' {
			_, w := utf8.DecodeRuneInString(l[j+1:])
			if j+1+w >= len(l) || l[j+1+w] != '\'' {
				return j, true // 'a: no closing quote follows
			}
		}
	}
	return skipString(l, j), true
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isIdent(c byte) bool {
	return c == '_' || isDigit(c) || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// rawStart opens a raw string at l[j] (r#"…"# in Rust, R"x(…)x" in C++);
// anything else is an identifier and not skipped.
func (x *lexer) rawStart(l string, j int) (int, bool) {
	if j > 0 && isIdent(l[j-1]) && !strings.ContainsRune("bLuU8", rune(l[j-1])) {
		return j, false
	}
	rest := l[j+1:]
	if x.lang.Raw == detect.RawRust {
		hashes := len(rest) - len(strings.TrimLeft(rest, "#"))
		if !strings.HasPrefix(rest[hashes:], `"`) {
			return j, false
		}
		x.raw = `"` + rest[:hashes]
		return j + hashes + 1, true
	}
	delim, _, ok := strings.Cut(strings.TrimPrefix(rest, `"`), "(")
	if !strings.HasPrefix(rest, `"`) || !ok || len(delim) > 16 || strings.ContainsAny(delim, ` )\\"`) {
		return j, false
	}
	x.raw = ")" + delim + `"`
	return j + 2 + len(delim), true
}

func skipString(l string, j int) int {
	q := l[j]
	for k := j + 1; k < len(l); k++ {
		switch l[k] {
		case '\\':
			k++
		case q:
			return k
		}
	}
	return len(l) // unterminated on this line (template literal): ignore rest
}

var pyBlock = lazyre.New(`^(\s*)(async\s+def|def|class)\b`)

// indentOutline elides bodies of Python functions nested only in classes,
// keeping the first docstring line.
func (o *Optimizer) indentOutline(lines []string) []span {
	elide := map[int]int{}
	type scope struct {
		indent int
		class  bool
	}
	var stack []scope
	for i := 0; i < len(lines); i++ {
		m := pyBlock.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent := len(m[1])
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		header := i
		for header < len(lines)-1 && !strings.HasSuffix(strings.TrimSpace(stripComment(lines[header])), ":") {
			header++ // multi-line signature
		}
		end := header
		for j := header + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) != "" && indentOf(lines[j]) <= indent {
				break
			}
			if strings.TrimSpace(lines[j]) != "" {
				end = j
			}
		}
		class := strings.Contains(m[2], "class")
		outer := !slices.ContainsFunc(stack, func(s scope) bool { return !s.class })
		if !class && outer {
			first := header + 1
			if first <= end && strings.HasPrefix(strings.TrimSpace(lines[first]), `"""`) {
				first++ // keep the docstring's first line
			}
			if end-first+1 >= o.cfg.MinBody {
				elide[first] = end
			}
		}
		stack = append(stack, scope{indent, class})
		i = header
	}
	return spans(elide)
}

func stripComment(l string) string {
	if i := strings.Index(l, " #"); i >= 0 {
		return l[:i]
	}
	return l
}

func indentOf(l string) int {
	return len(l) - len(strings.TrimLeft(l, " \t"))
}

// spans turns elided ranges (first → last line) into gaps, in line order.
func spans(elide map[int]int) []span {
	var out []span
	for from, to := range elide {
		if to >= from {
			out = append(out, span{from: from, to: to})
		}
	}
	slices.SortFunc(out, func(a, b span) int { return a.from - b.from })
	return out
}

// oneLiner reports whether a block opened on line t is also closed on it
// (def f, do: x; if x then y end).
var oneLiner = lazyre.New(`\bend\b[\s);,]*$|(^|[\s,])do:\s`)

// rowCache holds the patterns of language rows, so each compiles once instead
// of on every file.
var rowCache sync.Map // pattern -> *lazyre.Regexp

func rowRe(expr string) *lazyre.Regexp {
	if re, ok := rowCache.Load(expr); ok {
		return re.(*lazyre.Regexp)
	}
	re, _ := rowCache.LoadOrStore(expr, lazyre.New(expr))
	return re.(*lazyre.Regexp)
}

// endOutline elides bodies of functions in languages that close blocks with
// end (Ruby, Elixir, Lua, Julia): functions nested only in containers.
func (o *Optimizer) endOutline(lines []string, row detect.Language) []span {
	open, fn, cont := rowRe(row.Open), rowRe(row.Func), rowRe(row.Container)
	type block struct {
		line int
		kind byte // 'f' function, 'c' container, 'o' other
	}
	var stack []block
	elide := map[int]int{}
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "" || row.Line != "" && strings.HasPrefix(t, row.Line):
		case strings.HasPrefix(t, "end") && (len(t) == 3 || !isIdent(t[3])):
			if len(stack) == 0 {
				continue
			}
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			outer := !slices.ContainsFunc(stack, func(b block) bool { return b.kind != 'c' })
			if top.kind == 'f' && outer && i-top.line-1 >= o.cfg.MinBody {
				elide[top.line+1] = i - 1
			}
		case open.MatchString(l) && !oneLiner.MatchString(t):
			kind := byte('o')
			switch {
			case cont.MatchString(l) && row.Container != "":
				kind = 'c'
			case fn.MatchString(l) && row.Func != "":
				kind = 'f'
			}
			stack = append(stack, block{i, kind})
		}
	}
	return spans(elide)
}

// declOutline elides the lines under a declaration (deeper indented than it)
// for languages without block delimiters: Haskell, OCaml, Nim. Declarations
// nested only in containers (instance, module) are elided.
func (o *Optimizer) declOutline(lines []string, row detect.Language) []span {
	fn, cont := rowRe(row.Func), rowRe(row.Container)
	type scope struct {
		indent    int
		container bool
	}
	var stack []scope
	elide := map[int]int{}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if t == "" || row.Line != "" && strings.HasPrefix(t, row.Line) {
			continue
		}
		indent := indentOf(l)
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		switch {
		case cont.MatchString(l):
			stack = append(stack, scope{indent, true})
		case fn.MatchString(l):
			end := i
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) != "" && indentOf(lines[j]) <= indent {
					break
				}
				if strings.TrimSpace(lines[j]) != "" {
					end = j
				}
			}
			outer := !slices.ContainsFunc(stack, func(s scope) bool { return !s.container })
			if outer && end-i >= o.cfg.MinBody {
				elide[i+1] = end
			}
			i = end
		}
	}
	return spans(elide)
}

// parenOutline elides the inner lines of top-level forms (defn, define) in
// Lisp family languages; the first and the closing line stay.
func (o *Optimizer) parenOutline(lines []string, row detect.Language) []span {
	fn := rowRe(row.Func)
	elide := map[int]int{}
	for i := 0; i < len(lines); i++ {
		if !fn.MatchString(lines[i]) {
			continue
		}
		end := formEnd(lines, i)
		if end-i-1 >= o.cfg.MinBody {
			elide[i+1] = end - 1
		}
		i = end
	}
	return spans(elide)
}

// formEnd is the line where the form starting on line i closes, the last
// line when it never does. Strings and ; comments are skipped roughly.
func formEnd(lines []string, i int) int {
	depth := 0
	for j := i; j < len(lines); j++ {
		inStr := false
		l := lines[j]
		for k := 0; k < len(l); k++ {
			switch c := l[k]; {
			case c == '\\':
				k++
			case inStr:
				inStr = c != '"'
			case c == '"':
				inStr = true
			case c == ';':
				k = len(l)
			case c == '(' || c == '[' || c == '{':
				depth++
			case c == ')' || c == ']' || c == '}':
				depth--
			}
		}
		if depth <= 0 {
			return j
		}
	}
	return len(lines) - 1
}
