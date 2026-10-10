package k8soptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const (
	typ    = "k8s"
	family = "k8s"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{MaxItems: 15, MaxRows: 30, MaxCell: 60, NotesLines: 8} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
