package dboptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

const (
	typ    = "db"
	family = "db"
)

// NewFactory returns the factory that builds the "db" optimizer and puts it in the "db" family.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{Limits: table.Limits{MaxRows: 30, MaxCell: 80}, PlanMinShare: 0.02} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
