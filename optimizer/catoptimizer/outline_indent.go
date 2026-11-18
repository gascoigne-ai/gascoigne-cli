package catoptimizer

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var pyBlock = lazyre.New(`^(\s*)(async\s+def|def|class)\b`)

// indentOutline elides bodies of Python functions nested only in classes,
// keeping the first docstring line.
func (o *Optimizer) indentOutline(lines []string) []span {
	elide := map[int]int{}
	var stack []scope
	for i := 0; i < len(lines); i++ {
		m := pyBlock.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		indent := len(m[1])
		stack = popDeeper(stack, indent)
		header := i
		for header < len(lines)-1 && !strings.HasSuffix(strings.TrimSpace(stripComment(lines[header])), ":") {
			header++ // multi-line signature
		}
		end := bodyEnd(lines, header, indent)
		class := strings.Contains(m[2], "class")
		if !class && onlyContainers(stack) {
			o.elidePython(elide, lines, header, end)
		}
		stack = append(stack, scope{indent, class})
		i = header
	}
	return spans(elide)
}

// popDeeper drops the blocks indented at least as deep as indent: they ended.
func popDeeper(stack []scope, indent int) []scope {
	for len(stack) > 0 && stack[len(stack)-1].at >= indent {
		stack = stack[:len(stack)-1]
	}
	return stack
}

// bodyEnd is the last non-blank line after start that is indented deeper
// than indent; start itself when there is none.
func bodyEnd(lines []string, start, indent int) int {
	end := start
	for j := start + 1; j < len(lines); j++ {
		if strings.TrimSpace(lines[j]) == "" {
			continue
		}
		if indentOf(lines[j]) <= indent {
			break
		}
		end = j
	}
	return end
}

// elidePython marks the body of a function whose signature ends on line
// header and whose body ends on line end, keeping the docstring's first line.
func (o *Optimizer) elidePython(elide map[int]int, lines []string, header, end int) {
	first := header + 1
	if first <= end && strings.HasPrefix(strings.TrimSpace(lines[first]), `"""`) {
		first++ // keep the docstring's first line
	}
	if end-first+1 >= o.cfg.MinBody {
		elide[first] = end
	}
}

func stripComment(line string) string {
	if before, _, ok := strings.Cut(line, " #"); ok {
		return before
	}
	return line
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}
