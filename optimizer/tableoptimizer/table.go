// Package tableoptimizer is the table family engine: columnar CLI output
// (ps, ss, lsof, top, systemctl, docker ps, generic aligned or boxed
// tables) without padding or borders, constant columns hoisted, wide cells
// cut and long tables sampled keeping unhealthy rows.
package tableoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	query := sample.Terms(data.Attrs[payload.AttrQuery])
	return data.RewriteFamily(func(block payload.Block) (string, error) {
		if block.Type == detect.Eval {
			return compress.Join(table.FoldReport(compress.CleanLines(block.Text))), nil
		}
		t, ok := table.Parse(compress.CleanLines(block.Text))
		if !ok {
			data.Set(payload.AttrSkip, "table: no table found")
			return block.Text, nil
		}
		return compress.Join(table.Render(t, table.ForQuery(o.cfg.MaxRows, o.cfg.MaxCell, query))), nil
	}, detect.FamilyTable)
}
