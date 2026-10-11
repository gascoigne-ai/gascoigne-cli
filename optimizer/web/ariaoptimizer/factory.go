package ariaoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const (
	typ    = "aria"
	family = "web"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{MaxItems: 20, Head: 10, Tail: 4} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
