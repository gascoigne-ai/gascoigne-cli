package taskoptimizer

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	nxTask      = lazyre.New(`^\s*> nx run (\S+)`)
	nxCached    = lazyre.New(`^\s*✔\s+nx run (\S+)\s+\[(existing outputs match the cache|local cache|remote cache)`)
	nxFailed    = lazyre.New(`^\s*- nx run (\S+)`)
	nxSeparator = lazyre.New(`^\s*—{10,}`)
	nxRunning   = lazyre.New(`^\s*NX\s+Running target`)
	nxProject   = lazyre.New(`^- \S+$`)
)

// nx: "> nx run p:t" sections, cache lines, project list and a summary
// after the last separator.
func nx(lines []string) []string {
	scan := &nxScan{failed: failedNxTasks(lines)}
	for _, l := range lines {
		scan.line(l)
	}
	for i := range scan.sections {
		if !scan.sections[i].cached && !scan.sections[i].failed && hasFailure(scan.sections[i].body) {
			scan.sections[i].failed = true
		}
	}
	return render(scan.head, scan.sections, scan.tail)
}

// failedNxTasks names the tasks listed after the "targets failed" line.
func failedNxTasks(lines []string) map[string]bool {
	failed := map[string]bool{}
	inList := false
	for _, l := range lines {
		if strings.Contains(l, "targets failed") || strings.Contains(l, "Failed tasks:") {
			inList = true
		}
		if m := nxFailed.FindStringSubmatch(l); inList && m != nil {
			failed[m[1]] = true
		}
	}
	return failed
}

// nxScan is the state of nx while it reads lines: the header and tail
// text, the task sections and which of them the line belongs to.
type nxScan struct {
	failed           map[string]bool
	head, tail       []string
	sections         []section
	cur              *section
	inHeader, inTail bool
}

// line files one line under the header, a task or the tail.
func (scan *nxScan) line(line string) {
	switch {
	case nxRunning.MatchString(line):
		scan.head = append(scan.head, strings.TrimSpace(line))
		scan.inHeader = true
	case scan.inHeader && (nxProject.MatchString(line) || strings.TrimSpace(line) == ""):
	case nxSeparator.MatchString(line):
		scan.inHeader, scan.cur = false, nil
		scan.inTail = len(scan.sections) > 0
	case nxCached.MatchString(line):
		scan.sections = append(scan.sections, section{name: nxCached.FindStringSubmatch(line)[1], cached: true})
		scan.cur = nil
	case nxTask.MatchString(line):
		name := nxTask.FindStringSubmatch(line)[1]
		scan.sections = append(scan.sections, section{name: name, failed: scan.failed[name]})
		scan.cur, scan.inHeader = &scan.sections[len(scan.sections)-1], false
	case scan.cur != nil:
		scan.taskLine(line)
	case scan.inTail:
		scan.tail = append(scan.tail, line)
	default:
		scan.head = append(scan.head, line)
	}
}

// taskLine adds a line to the current task; the command echo that comes
// before any output ("> vite build") becomes its cmd.
func (scan *nxScan) taskLine(line string) {
	cur := scan.cur
	if c, ok := strings.CutPrefix(line, "> "); ok && cur.cmd == "" && strings.TrimSpace(strings.Join(cur.body, "")) == "" {
		cur.cmd = c
		return
	}
	cur.body = append(cur.body, line)
}
