// Package bypassfilter lets agents ask for unoptimized payloads up front,
// via the raw attr (CLI -raw, later HTTP header / MCP arg) or an env var.
package bypassfilter

import (
	"context"
	"os"
	"strconv"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

type Filter struct {
	component.Nop
	cfg *Config
}

func (f *Filter) Filter(_ context.Context, p *payload.Payload) (component.Decision, error) {
	if truthy(p.Attrs[payload.AttrRaw]) || (f.cfg.Env != "" && truthy(os.Getenv(f.cfg.Env))) {
		return component.Bypass, nil
	}
	return component.Continue, nil
}

func truthy(s string) bool {
	b, err := strconv.ParseBool(s)
	return err == nil && b
}
