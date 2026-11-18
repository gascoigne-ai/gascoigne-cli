// Package minsizefilter skips payloads too small to be worth optimizing.
package minsizefilter

import (
	"context"
	"fmt"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Filter bypasses payloads that are below the configured token count.
type Filter struct {
	component.Nop
	cfg *Config
}

func (f *Filter) Filter(_ context.Context, data *payload.Payload) (component.Decision, error) {
	if n := data.Tokens(); n < f.cfg.MinTokens {
		data.Set(payload.AttrSkip, fmt.Sprintf("%d tokens < min_tokens %d", n, f.cfg.MinTokens))
		return component.Bypass, nil
	}
	return component.Continue, nil
}
