package taskoptimizer

import (
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

var (
	turboLine   = lazyre.New(`^([\w@./-]+:[\w:-]+): ?(.*)$`)
	turboFailed = lazyre.New(`^\s*Failed:\s+(.+)$`)
	turboHead   = lazyre.New(`^• `)
)

// turbo: "pkg:task: line" prefixed output per task, "• ..." header and a
// Tasks/Cached/Time/Failed summary.
func turbo(lines []string) []string {
	failed := map[string]bool{}
	for _, l := range lines {
		if m := turboFailed.FindStringSubmatch(l); m != nil {
			for _, t := range strings.Fields(strings.ReplaceAll(m[1], ",", " ")) {
				failed[strings.Replace(t, "#", ":", 1)] = true
			}
		}
	}
	var head, tail []string
	var sections []section
	index := map[string]int{}
	for _, l := range lines {
		m := turboLine.FindStringSubmatch(l)
		switch {
		case turboHead.MatchString(l):
			head = append(head, l)
		case m != nil && !strings.Contains(m[1], "#"):
			i, ok := index[m[1]]
			if !ok {
				i = len(sections)
				index[m[1]] = i
				sections = append(sections, section{name: m[1], failed: failed[m[1]]})
			}
			s := &sections[i]
			body := m[2]
			switch {
			case strings.HasPrefix(body, "cache hit"):
				s.cached = true
			case strings.HasPrefix(body, "cache miss"), strings.HasPrefix(body, "cache bypass"):
			case strings.HasPrefix(body, "> ") && s.cmd == "" && !strings.Contains(body, "@"):
				s.cmd = strings.TrimPrefix(body, "> ")
			default:
				s.body = append(s.body, body)
			}
		default:
			tail = append(tail, l)
		}
	}
	for i := range sections {
		if hasFailure(sections[i].body) {
			sections[i].failed = true
		}
	}
	tail = slices.DeleteFunc(tail, func(l string) bool { return strings.Contains(l, "#") && strings.Contains(l, "exited (") })
	return render(head, sections, tail)
}
