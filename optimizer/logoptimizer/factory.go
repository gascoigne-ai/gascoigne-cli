package logoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "log"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{Context: 2, DropDebug: true, MaxLines: 300} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		})
}
