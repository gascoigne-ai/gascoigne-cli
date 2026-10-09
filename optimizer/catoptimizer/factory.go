package catoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "cat"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{Mode: Auto, CleanMinTokens: 800, OutlineMinTokens: 4000, MinBody: 4} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		})
}
