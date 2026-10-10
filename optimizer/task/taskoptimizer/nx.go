package taskoptimizer

import (
	"regexp"
	"strings"
)

var (
	nxTask      = regexp.MustCompile(`^\s*> nx run (\S+)`)
	nxCached    = regexp.MustCompile(`^\s*✔\s+nx run (\S+)\s+\[(existing outputs match the cache|local cache|remote cache)`)
	nxFailed    = regexp.MustCompile(`^\s*- nx run (\S+)`)
	nxSeparator = regexp.MustCompile(`^\s*—{10,}`)
	nxRunning   = regexp.MustCompile(`^\s*NX\s+Running target`)
	nxProject   = regexp.MustCompile(`^- \S+$`)
)

// nx: "> nx run p:t" sections, cache lines, project list and a summary
// after the last separator.
func nx(lines []string) []string {
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
	var head, tail []string
	var sections []section
	var cur *section
	inHeader, inTail := false, false
	for _, l := range lines {
		switch {
		case nxRunning.MatchString(l):
			head = append(head, strings.TrimSpace(l))
			inHeader = true
		case inHeader && (nxProject.MatchString(l) || strings.TrimSpace(l) == ""):
		case nxSeparator.MatchString(l):
			inHeader, cur = false, nil
			inTail = len(sections) > 0
		case nxCached.MatchString(l):
			sections = append(sections, section{name: nxCached.FindStringSubmatch(l)[1], cached: true})
		case nxTask.MatchString(l):
			name := nxTask.FindStringSubmatch(l)[1]
			sections = append(sections, section{name: name, failed: failed[name]})
			cur, inHeader = &sections[len(sections)-1], false
		case cur != nil:
			// The command echo comes before any output ("> vite build").
			if c, ok := strings.CutPrefix(l, "> "); ok && cur.cmd == "" && strings.TrimSpace(strings.Join(cur.body, "")) == "" {
				cur.cmd = c
				continue
			}
			cur.body = append(cur.body, l)
		case inTail:
			tail = append(tail, l)
		default:
			head = append(head, l)
		}
	}
	for i := range sections {
		if !sections[i].cached && !sections[i].failed && hasFailure(sections[i].body) {
			sections[i].failed = true
		}
	}
	return render(head, sections, tail)
}
