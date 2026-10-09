// Package minsizefilter skips payloads too small to be worth optimizing.
package minsizefilter

import (
	"context"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/payload"
)

type Filter struct {
	component.Nop
	cfg *Config
}

func (f *Filter) Filter(_ context.Context, p *payload.Payload) (component.Decision, error) {
	if p.Tokens() < f.cfg.MinTokens {
		return component.Bypass, nil
	}
	return component.Continue, nil
}
