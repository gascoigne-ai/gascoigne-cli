package infraoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "infra"
	family = "infra"
)

// NewFactory returns the factory that builds the "infra" optimizer and puts it in the "infra" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{MaxPlanLines: 300, DiffContext: 3} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
