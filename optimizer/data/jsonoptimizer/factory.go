package jsonoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const (
	typ    = "json"
	family = "data"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		defaultConfig,
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
