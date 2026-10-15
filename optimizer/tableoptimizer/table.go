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

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := sample.Terms(p.Attrs[payload.AttrQuery])
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyTable {
			continue
		}
		t, ok := table.Parse(compress.Lines(compress.StripANSI(b.Text)))
		if !ok {
			p.Set(payload.AttrSkip, "table: no table found")
			continue
		}
		p.Blocks[i].Text = compress.Join(table.Render(t, table.ForQuery(o.cfg.MaxRows, o.cfg.MaxCell, query)))
	}
	return nil
}
