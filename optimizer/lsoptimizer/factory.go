package lsoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "ls"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{SizesUpTo: 60, Width: 120} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		})
}
