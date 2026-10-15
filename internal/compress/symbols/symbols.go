// Package symbols finds definitions in source files with their line ranges,
// so an agent can read one function instead of the whole file: Go through
// its parser, Python by indentation, C-like languages (JS/TS, Java, C#,
// Rust, C, …) by definition keywords and brace matching.
package symbols

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Def is one definition: Name is qualified by its container ("Server.Run"),
// From and To are 1-based inclusive lines, doc comment included.
type Def struct {
	Name     string
	Kind     string // func, method, type, class
	From, To int
	Line     int // the signature line
}

// ErrLang means the language has no symbol strategy.
var ErrLang = fmt.Errorf("symbols: unsupported language")

// Defs lists the definitions of text, in file order.
func Defs(text string, h detect.Hints) ([]Def, error) {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	switch detect.Lang(text, h) {
	case detect.LangGo:
		return goDefs(text)
	case detect.LangPython:
		return pyDefs(lines), nil
	case detect.LangCLike:
		return braceDefs(lines), nil
	}
	return nil, ErrLang
}

// Find returns the definition named name: an exact qualified name, else a
// unique one whose last part matches ("Run" finds "Server.Run").
func Find(defs []Def, name string) (Def, error) {
	var hits []Def
	for _, d := range defs {
		if d.Name == name {
			return d, nil
		}
		if d.Name[strings.LastIndexByte(d.Name, '.')+1:] == name {
			hits = append(hits, d)
		}
	}
	switch len(hits) {
	case 0:
		return Def{}, fmt.Errorf("symbol %q not found", name)
	case 1:
		return hits[0], nil
	}
	names := make([]string, len(hits))
	for i, d := range hits {
		names[i] = d.Name
	}
	return Def{}, fmt.Errorf("symbol %q is ambiguous: %s", name, strings.Join(names, ", "))
}

func goDefs(src string) ([]Def, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	line := func(p token.Pos) int { return fset.Position(p).Line }
	var defs []Def
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			def := Def{Name: d.Name.Name, Kind: "func", From: line(d.Pos()), To: line(d.End()), Line: line(d.Pos())}
			if d.Recv != nil && len(d.Recv.List) > 0 {
				def.Name, def.Kind = recvName(d.Recv.List[0].Type)+"."+d.Name.Name, "method"
			}
			if d.Doc != nil {
				def.From = line(d.Doc.Pos())
			}
			defs = append(defs, def)
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, s := range d.Specs {
				ts := s.(*ast.TypeSpec)
				def := Def{Name: ts.Name.Name, Kind: "type", From: line(ts.Pos()), To: line(ts.End()), Line: line(ts.Pos())}
				if doc := cmpDoc(ts.Doc, d.Doc, len(d.Specs)); doc != nil {
					def.From = line(doc.Pos())
				}
				if len(d.Specs) == 1 {
					def.From, def.To = min(def.From, line(d.Pos())), line(d.End())
				}
				defs = append(defs, def)
			}
		}
	}
	return defs, nil
}

// cmpDoc is a type's own doc, else its declaration's when it declares one type.
func cmpDoc(own, decl *ast.CommentGroup, specs int) *ast.CommentGroup {
	if own != nil {
		return own
	}
	if specs == 1 {
		return decl
	}
	return nil
}

func recvName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return recvName(t.X)
	case *ast.IndexExpr:
		return recvName(t.X)
	case *ast.IndexListExpr:
		return recvName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "?"
}

var pyDef = lazyre.New(`^(\s*)(async\s+def|def|class)\s+(\w+)`)

func pyDefs(lines []string) []Def {
	type scope struct {
		indent int
		name   string
	}
	var defs []Def
	var stack []scope
	for i, l := range lines {
		m := pyDef.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		indent := len(m[1])
		for len(stack) > 0 && stack[len(stack)-1].indent >= indent {
			stack = stack[:len(stack)-1]
		}
		name := m[3]
		if len(stack) > 0 {
			name = stack[len(stack)-1].name + "." + name
		}
		kind := "func"
		if m[2] == "class" {
			kind = "class"
		} else if len(stack) > 0 {
			kind = "method"
		}
		from := i
		for from > 0 && strings.HasPrefix(strings.TrimSpace(lines[from-1]), "@") {
			from-- // decorators
		}
		defs = append(defs, Def{Name: name, Kind: kind, From: from + 1, To: pyEnd(lines, i, indent) + 1, Line: i + 1})
		stack = append(stack, scope{indent, name})
	}
	return defs
}

// pyEnd is the last non-blank line indented deeper than the definition.
func pyEnd(lines []string, i, indent int) int {
	end := i
	for j := i + 1; j < len(lines); j++ {
		t := strings.TrimSpace(lines[j])
		if t == "" {
			continue
		}
		if len(lines[j])-len(strings.TrimLeft(lines[j], " \t")) <= indent && !strings.HasPrefix(t, ")") {
			break
		}
		end = j
	}
	return end
}

var (
	// typeDef names a container: class Foo, struct bar, impl Baz for Qux, interface X.
	typeDef = lazyre.New(`\b(class|interface|struct|enum|trait|impl|record|namespace|object|type)\s+(?:<[^>]*>\s*)?(\w+)`)
	// funcDef names a function: fn run(, function run(, def run(, or a method
	// signature "Type name(args)" / "name(args) {".
	funcDef = lazyre.New(`\b(?:fn|function|func|def)\s+\*?\s*(\w+)\s*[<(]|^\s*(?:(?:public|private|protected|internal|static|async|override|virtual|abstract|final|export|default|pub(?:\([^)]*\))?|unsafe|const|inline|extern)\s+)*(?:[\w<>\[\],.?*&:]+\s+)*?(\w+)\s*(?:<[^>]*>)?\s*\([^;]*$`)
	// notFunc are words a call or control statement starts with.
	notFunc = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true, "return": true,
		"new": true, "else": true, "do": true, "try": true, "throw": true, "await": true, "sizeof": true, "typeof": true}
)

// braceDefs finds definitions whose body opens with { on the signature
// line or one of the next three, and closes at the matching brace.
func braceDefs(lines []string) []Def {
	var defs []Def
	type open struct {
		name string
		def  int // index in defs, -1 for other blocks
	}
	var stack []open
	pending := -1 // index in defs of a signature waiting for its {
	sigLine := 0
	for i, l := range lines {
		code := stripCode(l)
		if name, kind := defName(code); name != "" {
			qualified := name
			for k := len(stack) - 1; k >= 0; k-- {
				if stack[k].def >= 0 {
					qualified = defs[stack[k].def].Name + "." + name
					if kind == "func" {
						kind = "method"
					}
					break
				}
			}
			from := i
			for from > 0 && isDocLine(lines[from-1]) {
				from--
			}
			defs = append(defs, Def{Name: qualified, Kind: kind, From: from + 1, Line: i + 1, To: i + 1})
			pending, sigLine = len(defs)-1, i
		}
		for _, c := range code {
			switch c {
			case '{':
				o := open{def: -1}
				if pending >= 0 && i-sigLine <= 3 {
					o.def = pending
				}
				pending = -1
				stack = append(stack, o)
			case '}':
				if len(stack) == 0 {
					continue
				}
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if top.def >= 0 {
					defs[top.def].To = i + 1
				}
			case ';':
				pending = -1 // a declaration without body
			}
		}
	}
	return slices.DeleteFunc(defs, func(d Def) bool { return d.To == d.Line && !strings.Contains(stripCode(lines[d.Line-1]), "}") })
}

func defName(code string) (string, string) {
	// "type" also names parameters (Go-like signatures): only alone on its line.
	if m := typeDef.FindStringSubmatch(code); m != nil && (m[1] != "type" || !strings.Contains(code, "(")) {
		return m[2], "type"
	}
	m := funcDef.FindStringSubmatch(code)
	if m == nil {
		return "", ""
	}
	name := m[1] + m[2]
	if notFunc[name] || strings.Contains(code, "=") && !strings.Contains(code, "=>") {
		return "", ""
	}
	return name, "func"
}

func isDocLine(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*") ||
		strings.HasPrefix(t, "#[") || strings.HasPrefix(t, "@")
}

// stripCode drops strings and line comments, so braces inside them do not count.
func stripCode(l string) string {
	var b strings.Builder
	for j := 0; j < len(l); j++ {
		c := l[j]
		switch {
		case c == '/' && j+1 < len(l) && (l[j+1] == '/' || l[j+1] == '*'):
			return b.String()
		case c == '"' || c == '\'' || c == '`':
			q := c
			for j++; j < len(l) && l[j] != q; j++ {
				if l[j] == '\\' {
					j++
				}
			}
			b.WriteString(`""`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
