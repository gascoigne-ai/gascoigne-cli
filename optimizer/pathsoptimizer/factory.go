package pathsoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "paths"
	family = "core"
)

// NewFactory returns the factory that builds the "paths" optimizer and puts it in the "core" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		defaults,
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
