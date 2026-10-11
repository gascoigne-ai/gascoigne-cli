package docoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	typ    = "doc"
	family = "doc"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config {
			return &Config{MaxCodeLines: 40, ManDrop: []string{"SEE ALSO", "AUTHOR", "AUTHORS", "REPORTING BUGS", "COPYRIGHT", "HISTORY", "COLOPHON"}}
		},
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
