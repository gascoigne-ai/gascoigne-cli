package gooptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "go"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{} },
		func(context.Context, component.Settings, *Config) (component.Component, error) {
			return Optimizer{}, nil
		})
}
