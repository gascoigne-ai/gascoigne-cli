// Package traceoptimizer folds stack traces in command output (trace.go,
// frames.go): recursion cycles once with a count, goroutines of a Go
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
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Optimizer is the optimizer that folds stack traces: recursion cycles, goroutines on the same
// stack and runs of framework frames.
type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	for i, b := range data.Blocks {
		if !slices.Contains(o.cfg.Families, detect.Family(b.Type)) {
			continue
		}
		lines := compress.Lines(b.Text)
		folded := foldCycles(lines, o.cfg.MaxPeriod, o.cfg.MinRepeats)
		if strings.Contains(b.Text, "goroutine ") {
			folded = foldGoroutines(folded, o.cfg.MinGoroutines)
		}
		folded = foldFrames(folded, o.cfg.MinFrames)
		if len(folded) < len(lines) {
			data.Blocks[i].Text = compress.Join(folded)
		}
	}
	return nil
}
