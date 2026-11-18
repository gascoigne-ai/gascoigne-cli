package htmloptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "html"
	family = "web"
)

// NewFactory returns the factory that builds the "html" optimizer and puts it in the "web" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{Links: true} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
