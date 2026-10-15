// Package htmloptimizer turns web pages into markdown (compress/htmlx).
package htmloptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/htmlx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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
