package catoptimizer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// goOutline elides bodies of functions and methods using the Go parser.
// Signature and closing brace lines stay as they are.
func (o *Optimizer) goOutline(src string, lines []string) []span {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return o.braceOutline(lines)
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

var container = lazyre.New(`\b(class|interface|enum|struct|namespace|module|impl|trait|object|record)\b`)

// braceOutline elides bodies of blocks nested only in containers (classes,
// namespaces): functions and methods, top-level object literals. Strings and
// comments are skipped roughly; good enough for an outline.
func (o *Optimizer) braceOutline(lines []string) []span {
	type open struct {
		line      int
		container bool
	}
	var stack []open
	elide := map[int]int{} // first body line → last body line
	inBlockComment := false
	for i, l := range lines {
		// A block opens in a container when a keyword precedes its brace:
		// the line's first keyword ends before it. Found once per line, not
		// per brace, so minified one-line sources stay linear.
		kw := -1
		if strings.IndexByte(l, '{') >= 0 {
			if m := container.FindStringIndex(l); m != nil {
				kw = m[1]
			}
		}
		for j := 0; j < len(l); j++ {
			c := l[j]
			switch {
			case inBlockComment:
				if c == '*' && j+1 < len(l) && l[j+1] == '/' {
					inBlockComment = false
					j++
				}
			case c == '/' && j+1 < len(l) && l[j+1] == '/':
				j = len(l)
			case c == '/' && j+1 < len(l) && l[j+1] == '*':
				inBlockComment = true
				j++
			case c == '"' || c == '\'' || c == '`':
				j = skipString(l, j)
			case c == '{':
				stack = append(stack, open{i, kw >= 0 && kw <= j})
			case c == '}' && len(stack) > 0:
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
