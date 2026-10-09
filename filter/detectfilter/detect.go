// Package detectfilter tags each block with its content type, so optimizers
// can pick the blocks they understand. It never bypasses.
package detectfilter

import (
	"context"
	"log/slog"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Filter struct {
	component.Nop
	log *slog.Logger
}

func (f *Filter) Filter(_ context.Context, p *payload.Payload) (component.Decision, error) {
	h := detect.HintsFrom(p.Attrs)
	for i, b := range p.Blocks {
		if b.Type == "" {
			p.Blocks[i].Type = detect.Detect(b.Text, h)
			f.log.Debug("detected", "block", i, "type", p.Blocks[i].Type)
		}
	}
	return component.Continue, nil
}
