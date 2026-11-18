// Package detectfilter tags each block with its content type, so optimizers
// can pick the blocks they understand. It never bypasses.
package detectfilter

import (
	"context"
	"log/slog"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Filter struct {
	component.Nop
	cfg *Config
	log *slog.Logger
}

func (f *Filter) Filter(_ context.Context, data *payload.Payload) (component.Decision, error) {
	h := detect.HintsFrom(data.Attrs)
	for i, b := range data.Blocks {
		if b.Type == "" {
			data.Blocks[i].Type = f.editSafe(detect.Detect(b.Text, h), b.Text, h)
			f.log.Debug("detected", "block", i, "type", data.Blocks[i].Type)
		}
	}
	return component.Continue, nil
}

// editSafe types a config-sized file read from disk as source (never a lockfile:
// generated, not edited), so it is
// shrunk only by dropping whole comment lines.
func (f *Filter) editSafe(typ, text string, hints detect.Hints) string {
	if typ == detect.Lock || !detect.ReadsFile(hints.Cmd) || !slices.Contains(f.cfg.EditSafe, detect.Family(typ)) ||
		tokens.Estimate(text) >= f.cfg.EditSafeMaxTokens {
		return typ
	}
	return detect.Code
}
