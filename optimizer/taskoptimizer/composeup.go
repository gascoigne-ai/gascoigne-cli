package taskoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	composeLine  = lazyre.New(`^(\S+?)\s+\| (.*)$`)
	composeState = lazyre.New(`^\s*[✔✘⠿]\s+(Network|Container|Volume|Image)\s+(\S+)\s+(\w+)`)
)

// composeUp: docker compose up interleaves "service | line" logs; each
// service keeps errors with context plus its first and last lines;
// resource state lines collapse into counts.
func composeUp(lines []string) []string {
	parsed := parseCompose(lines)
	out := parsed.head
	if len(parsed.stateOrder) > 0 {
		var counts []string
		for _, k := range parsed.stateOrder {
			counts = append(counts, fmt.Sprintf("%s ×%d", k, parsed.states[k]))
		}
		out = append([]string{"resources: " + strings.Join(counts, ", ")}, out...)
	}
	for _, svc := range parsed.order {
		ls := compress.DedupeRuns(parsed.logs[svc])
		ls = compress.KeepWithContext(ls, func(line string) bool { return compress.SeverityOf(line) != compress.None }, 1, 2)
		out = append(out, svc+":")
		for _, l := range compress.HeadTail(ls, 3, 5) {
			out = append(out, "  "+l)
		}
	}
	return out
}

// composeOutput is docker compose up output cut into its parts.
type composeOutput struct {
	head       []string            // lines that are neither state nor service logs
	logs       map[string][]string // service -> its log lines
	order      []string            // services in order of appearance
	states     map[string]int      // "container started" -> count
	stateOrder []string
}

// parseCompose sorts the lines into resource states, service logs and the rest.
func parseCompose(lines []string) *composeOutput {
	parsed := &composeOutput{logs: map[string][]string{}, states: map[string]int{}}
	for _, l := range lines {
		if m := composeState.FindStringSubmatch(l); m != nil {
			k := strings.ToLower(m[1]) + " " + strings.ToLower(m[3])
			if parsed.states[k] == 0 {
				parsed.stateOrder = append(parsed.stateOrder, k)
			}
			parsed.states[k]++
			continue
		}
		if m := composeLine.FindStringSubmatch(l); m != nil {
			if _, ok := parsed.logs[m[1]]; !ok {
				parsed.order = append(parsed.order, m[1])
			}
			parsed.logs[m[1]] = append(parsed.logs[m[1]], m[2])
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(l), "[+]") {
			parsed.head = append(parsed.head, l)
		}
	}
	return parsed
}
