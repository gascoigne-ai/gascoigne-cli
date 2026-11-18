package proseoptimizer

import (
	"context"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/prose"
)

// NewFactory returns the factory that builds the "prose" optimizer and puts it in the "doc" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, "prose",
		func() *Config {
			return &Config{Device: "auto", Model: prose.Kompress, Threshold: 0.5, Warm: true, Idle: 5 * time.Minute, MinWords: 40}
		},
		func(_ context.Context, settings component.Settings, cfg *Config) (component.Component, error) {
			o := &Optimizer{cfg: cfg}
			if cfg.Enabled {
				o.model = &loaded{cfg: cfg, open: openModel(cfg), log: settings.Logger}
			}
			return o, nil
		},
		component.WithFamily("doc"))
}
