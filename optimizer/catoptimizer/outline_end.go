package catoptimizer

import (
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// oneLiner reports whether a block opened on line t is also closed on it
// (def f, do: x; if x then y end).
var oneLiner = lazyre.New(`\bend\b[\s);,]*$|(^|[\s,])do:\s`)

// endOutline elides bodies of functions in languages that close blocks with
// end (Ruby, Elixir, Lua, Julia): functions nested only in containers.
func (o *Optimizer) endOutline(lines []string, row detect.Language) []span {
	open, fn, cont := lazyre.Cached(row.Open), lazyre.Cached(row.Func), lazyre.Cached(row.Container)
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
			outer := !slices.ContainsFunc(stack, func(frame block) bool { return frame.kind != 'c' })
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
	fn, cont := lazyre.Cached(row.Func), lazyre.Cached(row.Container)
	var stack []scope
	elide := map[int]int{}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if t == "" || row.Line != "" && strings.HasPrefix(t, row.Line) {
			continue
		}
		indent := indentOf(l)
		stack = popDeeper(stack, indent)
		switch {
		case cont.MatchString(l):
			stack = append(stack, scope{indent, true})
		case fn.MatchString(l):
			end := bodyEnd(lines, i, indent)
			if onlyContainers(stack) && end-i >= o.cfg.MinBody {
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
	fn := lazyre.Cached(row.Func)
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
func formEnd(lines []string, index int) int {
	depth := 0
	for j := index; j < len(lines); j++ {
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
