// Package jsonoptimizer is the data family engine for JSON and NDJSON,
// built on compress/jsonx.
package jsonoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress/jsonx"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Config = jsonx.Config

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := rows.Terms(p.Attrs[payload.AttrQuery])
	return p.Rewrite(func(b payload.Block) (string, error) {
		return jsonx.Compress(b.Text, b.Type == detect.NDJSON, o.cfg, query)
	}, detect.JSON, detect.NDJSON)
}
