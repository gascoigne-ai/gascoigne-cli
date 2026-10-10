package taskoptimizer

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
)

var (
	composeLine  = regexp.MustCompile(`^(\S+?)\s+\| (.*)$`)
	composeState = regexp.MustCompile(`^\s*[✔✘⠿]\s+(Network|Container|Volume|Image)\s+(\S+)\s+(\w+)`)
)

// composeUp: docker compose up interleaves "service | line" logs; each
// service keeps errors with context plus its first and last lines;
// resource state lines collapse into counts.
func composeUp(lines []string) []string {
	var head []string
	logs := map[string][]string{}
	var order []string
	states := map[string]int{}
	var stateOrder []string
	for _, l := range lines {
		if m := composeState.FindStringSubmatch(l); m != nil {
			k := strings.ToLower(m[1]) + " " + strings.ToLower(m[3])
			if states[k] == 0 {
				stateOrder = append(stateOrder, k)
			}
			states[k]++
			continue
		}
		if m := composeLine.FindStringSubmatch(l); m != nil {
			if _, ok := logs[m[1]]; !ok {
				order = append(order, m[1])
			}
			logs[m[1]] = append(logs[m[1]], m[2])
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(l), "[+]") {
			head = append(head, l)
		}
	}
	var counts []string
	for _, k := range stateOrder {
		counts = append(counts, fmt.Sprintf("%s ×%d", k, states[k]))
	}
	out := head
	if len(counts) > 0 {
		out = append([]string{"resources: " + strings.Join(counts, ", ")}, out...)
	}
	for _, svc := range order {
		ls := compress.DedupeRuns(logs[svc])
		ls = compress.KeepWithContext(ls, func(l string) bool { return compress.SeverityOf(l) != compress.None }, 1, 2)
		out = append(out, svc+":")
		for _, l := range compress.HeadTail(ls, 3, 5) {
			out = append(out, "  "+l)
		}
	}
	return out
}
