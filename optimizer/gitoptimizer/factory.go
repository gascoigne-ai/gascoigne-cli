package gitoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "git"
	family = "vcs"
)

// NewFactory returns the factory that builds the "git" optimizer and puts it in the "vcs" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{ContextLines: 1, MaxFileLines: 200, MinMoveLines: 4} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
