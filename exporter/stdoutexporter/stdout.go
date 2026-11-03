// Package stdoutexporter writes payload text to stdout.
package stdoutexporter

import (
	"context"
	"github.com/gascoigne-ai/gascoigne-cli/internal/hint"
	"io"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// AttrHint set to HintTop prints the hint before the text instead of after
// it, for output that is cut from the end ("gascoigne exec … | head").
const (
	AttrHint = "hint"
	HintTop  = "top"
)

type Exporter struct {
	component.Nop
	cfg *Config
	w   io.Writer
}

func (e *Exporter) Export(_ context.Context, p *payload.Payload) error {
	text := p.Text()
	id := p.Attrs[payload.AttrRawID]
	if !e.cfg.Hint || id == "" {
		_, err := io.WriteString(e.w, text)
		return err
	}
	line := hint.Of(id) + "\n"
	if p.Attrs[AttrHint] == HintTop {
		_, err := io.WriteString(e.w, line+text)
		return err
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_, err := io.WriteString(e.w, text+line)
	return err
}
