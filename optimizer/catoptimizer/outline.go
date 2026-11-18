package catoptimizer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
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

// spans turns elided ranges (first → last line) into gaps, in line order.
func spans(elide map[int]int) []span {
	var out []span
	for from, to := range elide {
		if to >= from {
			out = append(out, span{from: from, to: to})
		}
	}
	slices.SortFunc(out, func(left, right span) int { return left.from - right.from })
	return out
}
