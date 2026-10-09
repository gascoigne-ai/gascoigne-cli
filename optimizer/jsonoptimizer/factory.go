package jsonoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "json"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config {
			return &Config{DropEmpty: true, MaxItems: 30, KeepEnds: 3, TabulateMin: 3, MaxString: 300, ShortenHashes: true}
		},
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		})
}
