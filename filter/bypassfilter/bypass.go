// Package bypassfilter lets agents ask for unoptimized payloads up front,
// via the raw attr (CLI -raw, later HTTP header / MCP arg) or an env var.
package bypassfilter

import (
	"context"
	"os"
	"strconv"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Filter struct {
	component.Nop
	cfg *Config
}

func (f *Filter) Filter(_ context.Context, data *payload.Payload) (component.Decision, error) {
	if v := data.Attrs[payload.AttrRaw]; truthy(v) {
		data.Set(payload.AttrSkip, "raw requested: attr raw="+v+" (-raw, MCP raw)")
		return component.Bypass, nil
	}
	if v := os.Getenv(f.cfg.Env); f.cfg.Env != "" && truthy(v) {
		data.Set(payload.AttrSkip, "raw requested: env "+f.cfg.Env+"="+v)
		return component.Bypass, nil
	}
	return component.Continue, nil
}

func truthy(text string) bool {
	b, err := strconv.ParseBool(text)
	return err == nil && b
}
