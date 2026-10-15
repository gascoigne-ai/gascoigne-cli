// Package traceoptimizer folds stack traces in command output (see
// compress/trace): recursion cycles once with a count, goroutines of a Go
// dump on the same stack once with the others' ids, runs of runtime and
// framework frames (JVM, Python, Rust, Ruby, .NET, PHP, gdb) as one count. It runs before the
// family engines, so they parse and cap the shorter text.
package traceoptimizer

import (
	"context"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/trace"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
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
		folded = trace.Frames(folded, o.cfg.MinFrames)
		if len(folded) < len(lines) {
			p.Blocks[i].Text = compress.Join(folded)
		}
	}
	return nil
}
