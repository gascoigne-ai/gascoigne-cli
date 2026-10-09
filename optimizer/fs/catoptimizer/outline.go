package catoptimizer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strings"
)

// goOutline elides bodies of functions and methods using the Go parser.
func (o *Optimizer) goOutline(src string) ([]string, int) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return o.braceOutline(strings.Split(strings.TrimSuffix(src, "\n"), "\n"))
	}
	type cut struct{ from, to, lines int }
	var cuts []cut
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		open, close := fset.Position(fn.Body.Lbrace), fset.Position(fn.Body.Rbrace)
		if n := close.Line - open.Line - 1; n >= o.cfg.MinBody {
			cuts = append(cuts, cut{open.Offset, close.Offset + 1, n})
		}
	}
	var b strings.Builder
	last := 0
	for _, c := range cuts {
		b.WriteString(src[last:c.from])
		b.WriteString("{ " + elision("", c.lines) + " }")
		last = c.to
	}
	b.WriteString(src[last:])
	return strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n"), len(cuts)
}

var container = regexp.MustCompile(`\b(class|interface|enum|struct|namespace|module|impl|trait|object|record)\b`)

// braceOutline elides bodies of blocks nested only in containers (classes,
// namespaces): functions and methods, top-level object literals. Strings and
// comments are skipped roughly; good enough for an outline.
func (o *Optimizer) braceOutline(lines []string) ([]string, int) {
	type open struct {
		line      int
		container bool
	}
	var stack []open
	elide := map[int]int{} // first body line → last body line
	inBlockComment := false
	for i, l := range lines {
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
				stack = append(stack, open{i, container.MatchString(l[:j])})
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
	return apply(lines, elide)
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

var pyBlock = regexp.MustCompile(`^(\s*)(async\s+def|def|class)\b`)

// indentOutline elides bodies of Python functions nested only in classes,
// keeping the first docstring line.
func (o *Optimizer) indentOutline(lines []string) ([]string, int) {
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
	return apply(lines, elide)
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

// apply replaces each elided range with one marker line.
func apply(lines []string, elide map[int]int) ([]string, int) {
	var out []string
	for i := 0; i < len(lines); i++ {
		if end, ok := elide[i]; ok && end >= i {
			indent := lines[i][:indentOf(lines[i])]
			out = append(out, elision(indent, end-i+1))
			i = end
			continue
		}
		out = append(out, lines[i])
	}
	return out, len(elide)
}
