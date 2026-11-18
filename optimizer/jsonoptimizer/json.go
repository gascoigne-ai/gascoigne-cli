// Package jsonoptimizer is the data family engine for JSON and NDJSON,
// built on compress/jsonx.
package jsonoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Config = jsonx.Config

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	query := sample.Terms(data.Attrs[payload.AttrQuery])
	return data.Rewrite(func(block payload.Block) (string, error) {
		return jsonx.Compress(block.Text, block.Type == detect.NDJSON, o.cfg, query)
	}, detect.JSON, detect.NDJSON)
}
