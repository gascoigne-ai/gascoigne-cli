package infraoptimizer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
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
	run := &ansibleRun{}
	for _, l := range lines {
		run.line(l)
	}
	run.flush()
	if run.quiet > 0 {
		run.out = append(run.out, compress.Omitted(run.quiet, "ok/skipped-only tasks"))
	}
	return run.out
}

// ansibleRun is the state of ansible: the task being read and what its
// result lines reported.
type ansibleRun struct {
	out     []string
	quiet   int // tasks that only reported ok or skipped
	task    string
	changed []string
	rest    []string
	failed  bool
	ok      int
}

// flush writes the task being read: in full when it failed, as one line
// when it changed hosts, else only counted.
func (run *ansibleRun) flush() {
	switch {
	case run.task == "":
	case run.failed:
		run.out = append(run.out, run.task)
		run.out = append(run.out, run.rest...)
	case len(run.changed) > 0:
		slices.Sort(run.changed)
		line := run.task + " changed: " + strings.Join(run.changed, ", ")
		if run.ok > 0 {
			line += fmt.Sprintf(" (ok/skipped ×%d)", run.ok)
		}
		run.out = append(run.out, line)
		run.out = append(run.out, run.rest...)
	default:
		run.quiet++
		run.out = append(run.out, run.rest...)
	}
	run.task, run.changed, run.rest, run.failed, run.ok = "", nil, nil, false, 0
}

// line reads one line: a task or play header, or a result of the task.
func (run *ansibleRun) line(line string) {
	if m := ansibleHeader.FindStringSubmatch(line); m != nil {
		run.flush()
		run.task = m[1] + " [" + m[2] + "]"
		return
	}
	if m := ansiblePlay.FindStringSubmatch(line); m != nil {
		run.flush()
		run.out = append(run.out, strings.TrimRight(line, "* "))
		return
	}
	if run.task == "" {
		run.out = append(run.out, line)
		return
	}
	run.result(line, ansibleResult.FindStringSubmatch(line))
}

// result reads a line under a task header; m is its ansibleResult match.
func (run *ansibleRun) result(line string, match []string) {
	switch {
	case match == nil:
		// Output under a result (-v dumps, msg lines) follows its kind;
		// warnings stay.
		if run.failed || strings.HasPrefix(line, "[WARNING]") || strings.HasPrefix(line, "[DEPRECATION WARNING]") {
			run.rest = append(run.rest, line)
		}
	case match[1] == "fatal" || match[1] == "failed" || match[1] == "unreachable":
		run.failed = true
		run.rest = append(run.rest, line)
	case match[1] == "changed":
		run.changed = append(run.changed, match[2])
	case match[1] == "ok", match[1] == "skipping", match[1] == "included":
		run.ok++
	}
}

var (
	cdkNoise = pcomb.Rules{
		{
			Name:  "cdk summary times",
			Match: pcomb.Prefixed("✨", pcomb.Indented(pcomb.Prefix("Synthesis time", "Total time", "Deployment time", "Number of stacks with differences"))),
			Yes:   []string{"✨  Synthesis time: 3.2s", "✨  Total time: 5.1s", "✨  Number of stacks with differences: 1"},
			No:    []string{"✨Total time", "✨  Done"},
		},
		{
			Name: "stack chatter",
			Match: afterLabel(pcomb.Prefix("deploying", "creating CloudFormation changeset", "building", "publishing", "asset built", "assets built", "asset published",
				"assets published")),
			Yes: []string{"MyStack: deploying... [1/2]", "MyStack: creating CloudFormation changeset...", "MyStack: building assets...", "MyStack: assets published"},
			No:  []string{"MyStack:deploying", "My Stack: deploying", "MyStack: deployed"},
		},
		{
			Name:  "asset progress",
			Match: pcomb.Re(`^\[\d+%\] (start|success|check|upload|build|cached)`),
			Yes:   []string{"[0%] start: Publishing abc:current", "[100%] success: Published abc:current", "[50%] check: Check abc"},
			No:    []string{"[%] start", "[5%]start", "[5%] failed: x"},
		},
	}
	// cdk deploy progress rows; failures are kept.
	cdkProgress = lazyre.New(`^\S+ \|\s*\d+/\d+ \|.*\| \w+_(IN_PROGRESS|COMPLETE)\s`)
)

// cdk drops synthesis, asset and progress chatter.
func cdk(lines []string) []string {
	var out []string
	progress := 0
	for _, l := range lines {
		switch {
		case cdkNoise.Match(l):
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
	changed := func(line string) bool {
		return strings.HasPrefix(line, "+ ") || strings.HasPrefix(line, "- ") || strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ")
	}
	return compress.KeepWithContext(lines, changed, o.cfg.DiffContext, o.cfg.DiffContext)
}
