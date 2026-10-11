// Package detectfilter tags each block with its content type, so optimizers
// can pick the blocks they understand. It never bypasses.
package detectfilter

import (
	"context"
	"log/slog"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Filter struct {
	component.Nop
	cfg *Config
	log *slog.Logger
}

func (f *Filter) Filter(_ context.Context, p *payload.Payload) (component.Decision, error) {
	h := detect.HintsFrom(p.Attrs)
	for i, b := range p.Blocks {
		if b.Type == "" {
			p.Blocks[i].Type = f.editSafe(detect.Detect(b.Text, h), b.Text, h)
			f.log.Debug("detected", "block", i, "type", p.Blocks[i].Type)
		}
	}
	return component.Continue, nil
}

// editSafe types a config-sized file read from disk as source, so it is
// shrunk only by dropping whole comment lines.
func (f *Filter) editSafe(typ, text string, h detect.Hints) string {
	if !detect.ReadsFile(h.Cmd) || !slices.Contains(f.cfg.EditSafe, detect.Family(typ)) ||
		tokens.Estimate(text) >= f.cfg.EditSafeMaxTokens {
		return typ
	}
	return detect.Code
}
