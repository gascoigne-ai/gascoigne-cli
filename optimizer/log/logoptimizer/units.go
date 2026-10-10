package logoptimizer

import "github.com/lukaszb/gascoigne/lazyre"

// unitLine is a systemd job message: "<time> host systemd[1]: Starting X...".
var unitLine = lazyre.New(`^[A-Z][a-z]{2} [ \d]\d \d\d:\d\d:\d\d (\S+ \S+?\[\d+\]): (Starting|Started|Finished|Stopping|Stopped) (.+?)(\.\.\.|\.)$`)

var (
	jobs     = map[string]string{"Starting": "start", "Stopping": "stop"}
	outcomes = map[string]string{"Started": "start", "Finished": "start", "Stopped": "stop"}
)

// unitPairs drops "Starting X..." and "Stopping X..." when the same
// process later reports X started, finished or stopped: the outcome line
// says it all. A job that never completes keeps its start line.
func unitPairs(lines []string) []string {
	done := map[string]bool{}
	drop := make([]bool, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		m := unitLine.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		key := m[1] + "\x00" + m[3] + "\x00"
		if job, ok := outcomes[m[2]]; ok {
			done[key+job] = true
		} else {
			drop[i] = done[key+jobs[m[2]]]
		}
	}
	var out []string
	for i, l := range lines {
		if !drop[i] {
			out = append(out, l)
		}
	}
	return out
}
