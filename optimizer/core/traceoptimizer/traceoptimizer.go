// Package traceoptimizer folds stack traces in command output (see
// compress/trace): recursion cycles once with a count, goroutines of a Go
// dump on the same stack once with the others' ids. It runs before the
// family engines, so they parse and cap the shorter text.
package traceoptimizer

import (
	"context"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/compress/trace"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if !slices.Contains(o.cfg.Families, detect.Family(b.Type)) {
			continue
		}
		lines := compress.Lines(b.Text)
		folded := trace.Cycles(lines, o.cfg.MaxPeriod, o.cfg.MinRepeats)
		if strings.Contains(b.Text, "goroutine ") {
			folded = trace.Goroutines(folded, o.cfg.MinGoroutines)
		}
		if len(folded) < len(lines) {
			p.Blocks[i].Text = compress.Join(folded)
		}
	}
	return nil
}
