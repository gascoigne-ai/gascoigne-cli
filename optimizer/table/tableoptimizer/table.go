// Package tableoptimizer is the table family engine: columnar CLI output
// (ps, ss, lsof, top, systemctl, docker ps, generic aligned or boxed
// tables) without padding or borders, constant columns hoisted, wide cells
// cut and long tables sampled keeping unhealthy rows.
package tableoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/compress/table"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := rows.Terms(p.Attrs[payload.AttrQuery])
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyTable {
			continue
		}
		t, ok := table.Parse(compress.Lines(compress.StripANSI(b.Text)))
		if !ok {
			p.Set(payload.AttrSkip, "table: no table found")
			continue
		}
		p.Blocks[i].Text = compress.Join(table.Render(t, table.Options{
			MaxRows: o.cfg.MaxRows, MaxCell: o.cfg.MaxCell,
			Rows: rows.Options{HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2, RelevanceK: 3, Query: query},
		}))
	}
	return nil
}
