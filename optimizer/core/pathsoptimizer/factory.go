package pathsoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const (
	typ    = "paths"
	family = "core"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		defaults,
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
