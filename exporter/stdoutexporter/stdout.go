// Package stdoutexporter writes payload text to stdout.
package stdoutexporter

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
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
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	_, err := fmt.Fprintf(e.w, "%s[gascoigne: optimized %s→%s tokens · full: gascoigne raw %s]\n",
		text, p.Attrs[payload.AttrRawTokens], p.Attrs[payload.AttrOutTokens], id)
	return err
}
