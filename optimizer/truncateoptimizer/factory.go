package truncateoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "truncate"
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
