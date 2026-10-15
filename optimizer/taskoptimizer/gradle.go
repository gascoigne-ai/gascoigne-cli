package taskoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	gradleTask  = lazyre.New(`^> Task (:\S+)(?: (UP-TO-DATE|NO-SOURCE|FROM-CACHE|SKIPPED|FAILED))?$`)
	gradleNoise = lazyre.New(`^(Downloading https?://|Download https?://|Starting a Gradle Daemon|<[-=]+> \d+% |Deprecated Gradle features were used|You can use '--warning-mode all'|For more on this, please refer to|\* Get more help at|> Run with --(stacktrace|info|debug|scan)|> Get more help|Configuration cache entry)`)
)

// gradle: "> Task :x" sections; quiet up-to-date/cached tasks are counted.
func gradle(lines []string) []string {
	var head, tail []string
	var sections []section
	status := map[string]int{}
	var cur *section
	for _, l := range lines {
		switch m := gradleTask.FindStringSubmatch(l); {
		case gradleNoise.MatchString(strings.TrimSpace(l)):
		case m != nil:
			state := m[2]
			if state == "" {
				state = "executed"
			}
			status[state]++
			sections = append(sections, section{name: m[1], failed: state == "FAILED", cached: state != "executed" && state != "FAILED"})
			cur = &sections[len(sections)-1]
		case strings.HasPrefix(l, "BUILD ") || strings.HasPrefix(l, "FAILURE:") || strings.HasPrefix(l, "* What went wrong") || strings.HasPrefix(l, "* Try:") || strings.HasPrefix(l, "* Exception"):
			cur = nil
			tail = append(tail, l)
		case cur != nil:
			cur.body = append(cur.body, l)
		case len(sections) == 0:
			head = append(head, l)
		default:
			tail = append(tail, l)
		}
	}
	// Executed tasks with output (warnings) are worth showing; silent ones are not.
	var shown []section
	for _, s := range sections {
		if s.failed || len(strings.TrimSpace(strings.Join(s.body, ""))) > 0 && hasFailure(s.body) {
			s.failed = true
			shown = append(shown, s)
		}
	}
	var counts []string
	for _, st := range []string{"executed", "UP-TO-DATE", "FROM-CACHE", "NO-SOURCE", "SKIPPED", "FAILED"} {
		if n := status[st]; n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, strings.ToLower(st)))
		}
	}
	if len(counts) > 0 {
		head = append(head, "tasks: "+strings.Join(counts, ", "))
	}
	return render(head, shown, tail)
}
