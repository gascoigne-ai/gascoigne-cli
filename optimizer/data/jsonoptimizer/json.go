// Package jsonoptimizer is the data family engine for JSON and NDJSON,
// built on compress/jsonx.
package jsonoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Config = jsonx.Config

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := sample.Terms(p.Attrs[payload.AttrQuery])
	return p.Rewrite(func(b payload.Block) (string, error) {
		return jsonx.Compress(b.Text, b.Type == detect.NDJSON, o.cfg, query)
	}, detect.JSON, detect.NDJSON)
}
