// Package taskoptimizer is the task family engine: task runners and CI logs
// (nx, turbo, make, gradle, just, docker compose up, GitHub Actions logs).
// Output splits into tasks or steps; successful and cached ones collapse to
// a line, failing ones keep their output compressed by the family it
// belongs to (test, build, pkg) via compress/compose.
package taskoptimizer

import (
	"context"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/compose"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct{ component.Nop }

var splitters = map[string]func([]string) []string{
	"nx":      nx,
	"turbo":   turbo,
	"make":    makeLike,
	"just":    makeLike,
	"gradle":  gradle,
	"compose": composeUp,
	"ci":      ciLog,
}

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyTask {
			continue
		}
		split, ok := splitters[detect.Dialect(b.Type)]
		if !ok {
			continue
		}
		p.Blocks[i].Text = compress.Join(compress.SqueezeBlank(split(compress.Lines(compress.StripANSI(b.Text)))))
	}
	return nil
}

// section is one task or step.
type section struct {
	name   string
	cmd    string // command it ran, a hint for detection
	body   []string
	failed bool
	cached bool
}

// render prints collapsed successes then failing sections compressed by
// their own family.
func render(head []string, sections []section, tail []string) []string {
	var cached, ok []string
	var failed []section
	for _, s := range sections {
		switch {
		case s.failed:
			failed = append(failed, s)
		case s.cached:
			cached = append(cached, s.name)
		default:
			ok = append(ok, s.name)
		}
	}
	out := head
	if len(cached) > 0 {
		out = append(out, "cached: "+strings.Join(cached, ", "))
	}
	if len(ok) > 0 {
		out = append(out, "succeeded: "+strings.Join(ok, ", "))
	}
	for _, s := range failed {
		out = append(out, "", "failed: "+s.name)
		out = append(out, compose.Lines(s.body, s.cmd)...)
	}
	return append(out, tail...)
}

// hasFailure reports error signs in a section body.
func hasFailure(body []string) bool {
	for _, l := range body {
		if strings.Contains(l, "exited with non-zero status") || strings.Contains(l, "exited (") && !strings.Contains(l, "exited (0)") ||
			compress.SeverityOf(l) == compress.Error {
			return true
		}
	}
	return false
}
