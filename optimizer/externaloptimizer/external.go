// Package externaloptimizer runs a user's own program as an optimizer: any
// script or binary that reads a block and prints a shorter one. Three
// protocols (text, json, rpc) are described in docs/external-optimizer.md.
//
// The child is untrusted for correctness, not for security: it runs with the
// user's rights but a scrubbed environment. Every failure (timeout, bad exit,
// bad answer) is an error, and core then returns the original payload. A
// rewrite that is not smaller is dropped, so a child cannot make output
// bigger. Originals stay retrievable through the ledger as for any optimizer.
package externaloptimizer

import (
	"context"
	"fmt"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Optimizer struct {
	cfg   *Config
	id    component.ID
	codec codec
}

func (o *Optimizer) Start(context.Context, component.Host) error { return nil }

func (o *Optimizer) Shutdown(context.Context) error {
	o.codec.close()
	return nil
}

func (o *Optimizer) Optimize(ctx context.Context, p *payload.Payload) error {
	cmd := p.Attrs[payload.AttrCmd]
	var skip string
	changed := false
	for i, b := range p.Blocks {
		if b.Text == "" || !o.cfg.wants(b.Type, cmd) || o.tooBig(b.Text) {
			continue
		}
		text, why, err := o.rewrite(ctx, b, p.Attrs)
		if err != nil {
			return fmt.Errorf("%s: %w", o.id, err)
		}
		if skip == "" {
			skip = why
		}
		if text != b.Text {
			p.Blocks[i].Text, changed = text, true
		}
	}
	if !changed {
		p.Set(payload.AttrSkip, o.skipReason(skip))
	}
	return nil
}

func (o *Optimizer) tooBig(text string) bool {
	return o.cfg.MaxInputTokens > 0 && tokens.Estimate(text) > o.cfg.MaxInputTokens
}

// rewrite asks the child about one block. A reply that is empty for non-empty
// input is an error; one that is not smaller is ignored.
func (o *Optimizer) rewrite(ctx context.Context, b payload.Block, attrs map[string]string) (text, why string, err error) {
	req := Request{Version: Version, Attrs: attrs, Block: Block{Kind: string(b.Kind), Type: b.Type, Text: b.Text}}
	resp, err := o.codec.call(ctx, req)
	if err != nil {
		return "", "", err
	}
	text, err = resp.check()
	switch {
	case err != nil:
		return "", "", err
	case text == "":
		return "", "", fmt.Errorf("child returned empty text for %d bytes of input", len(b.Text))
	case len(text) > o.cfg.MaxOutputBytes:
		return "", "", errTooBig
	case tokens.Estimate(text) >= tokens.Estimate(b.Text):
		return b.Text, resp.SkipReason, nil
	}
	return text, resp.SkipReason, nil
}

func (o *Optimizer) skipReason(childWhy string) string {
	if childWhy != "" {
		return fmt.Sprintf("%s: %s", o.id, childWhy)
	}
	return fmt.Sprintf("%s: nothing shorter returned", o.id)
}
