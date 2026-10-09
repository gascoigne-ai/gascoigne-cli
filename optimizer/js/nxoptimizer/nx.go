// Package nxoptimizer compresses nx task runner output: successful and
// cached tasks collapse to one line each, failing tasks keep their output
// (with passing tests and dependency stack frames dropped), and the final
// summary stays.
package nxoptimizer

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct{ component.Nop }

var (
	taskHeader = regexp.MustCompile(`^\s*> nx run (\S+)`)
	cached     = regexp.MustCompile(`^\s*✔\s+nx run (\S+)\s+\[(existing outputs match the cache|local cache|remote cache)`)
	failedTask = regexp.MustCompile(`^\s*- nx run (\S+)`)
	separator  = regexp.MustCompile(`^\s*—{10,}`)
	running    = regexp.MustCompile(`^\s*NX\s+Running target`)
	project    = regexp.MustCompile(`^- \S+$`)
)

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		return compress.Join(compress.SqueezeBlank(rewrite(compress.Lines(compress.StripANSI(b.Text))))), nil
	}, detect.NX)
}

func rewrite(lines []string) []string {
	failed := failedTasks(lines)
	var out, cache, ok []string
	var task string
	var body []string
	flush := func() {
		switch {
		case task == "":
		case failed[task] || hasError(body):
			out = append(out, "> nx run "+task)
			if compress.HasJSTest(strings.Join(body, "\n")) {
				body = compress.JSTest(body)
			} else {
				body = dropDepFrames(body)
			}
			out = append(out, body...)
		default:
			ok = append(ok, task)
		}
		task, body = "", nil
	}
	inHeader := false
	for _, l := range lines {
		switch {
		case running.MatchString(l):
			out = append(out, strings.TrimSpace(l))
			inHeader = true
		case inHeader && (project.MatchString(l) || strings.TrimSpace(l) == ""):
		case separator.MatchString(l):
			flush()
			inHeader = false
		case cached.MatchString(l):
			cache = append(cache, cached.FindStringSubmatch(l)[1])
		case taskHeader.MatchString(l):
			flush()
			inHeader = false
			task = taskHeader.FindStringSubmatch(l)[1]
		case task != "":
			body = append(body, l)
		default:
			out = append(out, l)
		}
	}
	flush()
	var summary []string
	if len(cache) > 0 {
		summary = append(summary, "cached: "+strings.Join(cache, ", "))
	}
	if len(ok) > 0 {
		summary = append(summary, "succeeded: "+strings.Join(ok, ", "))
	}
	// Task results go right after the "Running target" line.
	at := slices.IndexFunc(out, running.MatchString) + 1
	return slices.Concat(out[:at], summary, out[at:])
}

func failedTasks(lines []string) map[string]bool {
	failed := map[string]bool{}
	inList := false
	for _, l := range lines {
		if strings.Contains(l, "targets failed") || strings.Contains(l, "Failed tasks:") {
			inList = true
			continue
		}
		if m := failedTask.FindStringSubmatch(l); inList && m != nil {
			failed[m[1]] = true
		}
	}
	return failed
}

func hasError(lines []string) bool {
	for _, l := range lines {
		if strings.Contains(l, "exited with non-zero status") || compress.SeverityOf(l) == compress.Error {
			return true
		}
	}
	return false
}

var depFrame = regexp.MustCompile(`^\s*at .*(node_modules|node:internal)`)

func dropDepFrames(lines []string) []string {
	var out []string
	n := 0
	for _, l := range lines {
		if depFrame.MatchString(l) {
			n++
			continue
		}
		if n > 0 {
			out = append(out, compress.Omitted(n, "dependency stack frames"))
			n = 0
		}
		out = append(out, l)
	}
	if n > 0 {
		out = append(out, compress.Omitted(n, "dependency stack frames"))
	}
	return out
}
