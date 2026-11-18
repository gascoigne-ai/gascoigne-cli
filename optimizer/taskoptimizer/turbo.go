package taskoptimizer

import (
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	turboLine   = lazyre.New(`^([\w@./-]+:[\w:-]+): ?(.*)$`)
	turboFailed = lazyre.New(`^\s*Failed:\s+(.+)$`)
	turboHead   = lazyre.New(`^• `)
)

// turbo: "pkg:task: line" prefixed output per task, "• ..." header and a
// Tasks/Cached/Time/Failed summary.
func turbo(lines []string) []string {
	failed := turboFailedTasks(lines)
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
			addTurboLine(&sections[i], m[2])
		default:
			tail = append(tail, l)
		}
	}
	for i := range sections {
		if hasFailure(sections[i].body) {
			sections[i].failed = true
		}
	}
	tail = slices.DeleteFunc(tail, func(line string) bool { return strings.Contains(line, "#") && strings.Contains(line, "exited (") })
	return render(head, sections, tail)
}

// turboFailedTasks names the tasks of the "Failed:" summary as pkg:task.
func turboFailedTasks(lines []string) map[string]bool {
	failed := map[string]bool{}
	for _, l := range lines {
		if m := turboFailed.FindStringSubmatch(l); m != nil {
			for t := range strings.FieldsSeq(strings.ReplaceAll(m[1], ",", " ")) {
				failed[strings.Replace(t, "#", ":", 1)] = true
			}
		}
	}
	return failed
}

// addTurboLine files the text of a prefixed line under its task: cache
// status and the command echo are kept apart from the output.
func addTurboLine(task *section, body string) {
	switch {
	case strings.HasPrefix(body, "cache hit"):
		task.cached = true
	case strings.HasPrefix(body, "cache miss"), strings.HasPrefix(body, "cache bypass"):
	case strings.HasPrefix(body, "> ") && task.cmd == "" && !strings.Contains(body, "@"):
		task.cmd = strings.TrimPrefix(body, "> ")
	default:
		task.body = append(task.body, body)
	}
}
