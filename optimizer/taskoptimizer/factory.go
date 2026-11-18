package taskoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "task"
	family = "task"
)

// NewFactory returns the factory that builds the "task" optimizer and puts it in the "task" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{} },
		func(context.Context, component.Settings, *Config) (component.Component, error) {
			return Optimizer{}, nil
		},
		component.WithFamily(family))
}
