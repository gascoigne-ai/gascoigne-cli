package grepoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "grep"
	family = "search"
)

// NewFactory returns the factory that builds the "grep" optimizer and puts it in the "search" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{MaxPerFile: 20, MaxFiles: 100, MaxLineLen: 160} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
