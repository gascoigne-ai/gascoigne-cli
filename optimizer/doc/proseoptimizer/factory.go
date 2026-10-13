package proseoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, "prose",
		func() *Config { return &Config{Threshold: 0.5, Warm: true, MinWords: 40} },
		func(_ context.Context, s component.Settings, cfg *Config) (component.Component, error) {
			o := &Optimizer{cfg: cfg}
			if cfg.Enabled {
				o.helper = &helper{cfg: cfg, log: s.Logger}
			}
			return o, nil
		},
		component.WithFamily("doc"))
}
