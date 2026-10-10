package infraoptimizer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/lazyre"
)

var (
	pulumiSpinner = lazyre.New(`^@ \w+\.*$`)
	// In-progress steps are repeated once they finish.
	pulumiPending = lazyre.New(`^ [+~-]{1,2}\s.*\s(creating|updating|deleting|replacing|refreshing|reading) \(\d+s\)`)
	// Resource rows without an operation: unchanged.
	pulumiSame = lazyre.New(`^\s{3,}([│├└─ ]+)?[\w-]+:[\w/-]*:[\w/-]+\s+\S+(\s+running)?\s*$`)
)

// pulumi drops spinners, in-progress steps and unchanged resources.
func pulumi(lines []string) []string {
	var out []string
	same := 0
	for _, l := range lines {
		switch {
		case pulumiSpinner.MatchString(l), pulumiPending.MatchString(l):
		case pulumiSame.MatchString(l):
			same++
		default:
			out = append(out, l)
		}
	}
	if same > 0 {
		out = append(out, compress.Omitted(same, "unchanged resource rows"))
	}
	return out
}

var (
	ansibleHeader = lazyre.New(`^(TASK|RUNNING HANDLER) \[(.*)\] \**$`)
	ansiblePlay   = lazyre.New(`^(PLAY( RECAP)?)( \[.*\])? \**$`)
	ansibleResult = lazyre.New(`^(ok|changed|skipping|fatal|failed|unreachable|included): \[([^\]]+)\]`)
)

// ansible keeps plays, failures and the recap; tasks that only reported ok
// or skipped are counted, changed tasks become one line naming the hosts.
func ansible(lines []string) []string {
	var out []string
	quiet := 0
	var task string
	var changed, rest []string
	failed := false
	ok := 0
	flush := func() {
		switch {
		case task == "":
		case failed:
			out = append(out, task)
			out = append(out, rest...)
		case len(changed) > 0:
			slices.Sort(changed)
			line := task + " changed: " + strings.Join(changed, ", ")
			if ok > 0 {
				line += fmt.Sprintf(" (ok/skipped ×%d)", ok)
			}
			out = append(out, line)
			out = append(out, rest...)
		default:
			quiet++
			out = append(out, rest...)
		}
		task, changed, rest, failed, ok = "", nil, nil, false, 0
	}
	for _, l := range lines {
		if m := ansibleHeader.FindStringSubmatch(l); m != nil {
			flush()
			task = m[1] + " [" + m[2] + "]"
			continue
		}
		if m := ansiblePlay.FindStringSubmatch(l); m != nil {
			flush()
			out = append(out, strings.TrimRight(l, "* "))
			continue
		}
		m := ansibleResult.FindStringSubmatch(l)
		switch {
		case task == "":
			out = append(out, l)
		case m == nil:
			// Output under a result (-v dumps, msg lines) follows its kind;
			// warnings stay.
			if failed || strings.HasPrefix(l, "[WARNING]") || strings.HasPrefix(l, "[DEPRECATION WARNING]") {
				rest = append(rest, l)
			}
		case m[1] == "fatal" || m[1] == "failed" || m[1] == "unreachable":
			failed = true
			rest = append(rest, l)
		case m[1] == "changed":
			changed = append(changed, m[2])
		case m[1] == "ok", m[1] == "skipping", m[1] == "included":
			ok++
		}
	}
	flush()
	if quiet > 0 {
		out = append(out, compress.Omitted(quiet, "ok/skipped-only tasks"))
	}
	return out
}

var (
	cdkNoise = lazyre.New(`^(✨\s+(Synthesis time|Total time|Deployment time|Number of stacks with differences)|\S+: (deploying|creating CloudFormation changeset|building|publishing|assets? (built|published))|\[\d+%\] (start|success|check|upload|build|cached))`)
	// cdk deploy progress rows; failures are kept.
	cdkProgress = lazyre.New(`^\S+ \|\s*\d+/\d+ \|.*\| \w+_(IN_PROGRESS|COMPLETE)\s`)
)

// cdk drops synthesis, asset and progress chatter.
func cdk(lines []string) []string {
	var out []string
	progress := 0
	for _, l := range lines {
		switch {
		case cdkNoise.MatchString(l):
		case cdkProgress.MatchString(l):
			progress++
		default:
			out = append(out, l)
		}
	}
	if progress > 0 {
		out = append(out, compress.Omitted(progress, "progress events"))
	}
	return out
}

// helmDiff keeps resource headers and changed lines with some context.
func (o *Optimizer) helmDiff(lines []string) []string {
	changed := func(l string) bool {
		return strings.HasPrefix(l, "+ ") || strings.HasPrefix(l, "- ") || strings.HasSuffix(l, ":") && !strings.HasPrefix(l, " ")
	}
	return compress.KeepWithContext(lines, changed, o.cfg.DiffContext, o.cfg.DiffContext)
}
