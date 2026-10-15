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

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		jsonx.Default,
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
