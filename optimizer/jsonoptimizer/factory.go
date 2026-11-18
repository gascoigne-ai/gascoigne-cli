package jsonoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

const (
	typ    = "json"
	family = "data"
)

// NewFactory returns the factory that builds the "json" optimizer and puts it in the "data" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		jsonx.Default,
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
