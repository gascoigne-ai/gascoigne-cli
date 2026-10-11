package testoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const (
	typ    = "test"
	family = "test"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{CoverageBelow: 80} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
