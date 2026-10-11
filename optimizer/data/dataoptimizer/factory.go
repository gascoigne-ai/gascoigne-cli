package dataoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress/jsonx"
)

const (
	typ    = "data"
	family = "data"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config {
			return &Config{JSON: *jsonx.Default(), MaxRows: 30, MaxCell: 60, MaxItems: 20, MaxOutputLines: 30}
		},
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
