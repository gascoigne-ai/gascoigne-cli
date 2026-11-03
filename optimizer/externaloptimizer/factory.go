package externaloptimizer

import (
	"context"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "external"
	family = "external"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config {
			return &Config{Protocol: JSON, Timeout: 5 * time.Second, MaxOutputBytes: 8 << 20}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg, id: set.ID, codec: codecs[cfg.Protocol](cfg, set.Logger)}, nil
		},
		component.WithFamily(family))
}
