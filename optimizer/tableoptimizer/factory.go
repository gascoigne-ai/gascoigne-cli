package tableoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

const (
	typ    = "table"
	family = "table"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{Limits: table.Limits{MaxRows: 30, MaxCell: 60}} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
