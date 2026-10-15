// Package proseoptimizer drops low-value words from the plain paragraphs of
// doc/text and doc/markdown with a local model, loaded in this process.
// It is opt-in and lossy: the original stays in the ledger. Its own optimizer
// id keeps its savings apart in `gain -by optimizer` and the dashboard.
package proseoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg   *Config
	model *loaded // nil unless enabled
}

// Start loads the model in the background so the long-lived server answers
// its first prose paragraph warm.
func (o *Optimizer) Start(context.Context, component.Host) error {
	if o.model != nil && o.cfg.Warm {
		go o.model.warm()
	}
	return nil
}

func (o *Optimizer) Shutdown(context.Context) error {
	if o.model != nil {
		o.model.close()
	}
	return nil
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	if o.model == nil {
		return nil
	}
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyDoc {
			continue
		}
		if d := detect.Dialect(b.Type); d != "text" && d != "markdown" {
			continue
		}
		p.Blocks[i].Text = compress.Join(compress.SqueezeBlank(o.prose(compress.Lines(b.Text))))
	}
	return nil
}
