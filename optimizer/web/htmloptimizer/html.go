// Package htmloptimizer turns web pages into markdown (compress/htmlx).
package htmloptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress/htmlx"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if b.Type != detect.HTML {
			continue
		}
		md, err := htmlx.Markdown(b.Text, o.cfg.Links)
		if err != nil {
			return err
		}
		p.Blocks[i].Text = md
		p.Blocks[i].Type = detect.Markdown
	}
	return nil
}
