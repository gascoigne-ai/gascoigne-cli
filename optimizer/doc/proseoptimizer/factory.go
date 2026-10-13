package proseoptimizer

import (
	"context"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, "prose",
		func() *Config {
			return &Config{Device: "auto", Threshold: 0.5, Warm: true, Idle: 5 * time.Minute, MinWords: 40}
		},
		func(_ context.Context, s component.Settings, cfg *Config) (component.Component, error) {
			o := &Optimizer{cfg: cfg}
			if cfg.Enabled {
				o.model = &loaded{cfg: cfg, open: openModel(cfg), log: s.Logger}
			}
			return o, nil
		},
		component.WithFamily("doc"))
}
