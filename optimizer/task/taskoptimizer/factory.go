package taskoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const (
	typ    = "task"
	family = "task"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{} },
		func(context.Context, component.Settings, *Config) (component.Component, error) {
			return Optimizer{}, nil
		},
		component.WithFamily(family))
}
