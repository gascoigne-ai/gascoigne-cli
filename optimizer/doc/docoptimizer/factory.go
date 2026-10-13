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
			return &Config{MaxCodeLines: 40, ManDrop: []string{"SEE ALSO", "AUTHOR", "AUTHORS", "REPORTING BUGS", "COPYRIGHT", "HISTORY", "COLOPHON"},
				Model: Model{Threshold: 0.5, Warm: true, MinWords: 40}}
		},
		func(_ context.Context, s component.Settings, cfg *Config) (component.Component, error) {
			o := &Optimizer{cfg: cfg}
			if cfg.Model.Enabled {
				o.helper = &helper{cfg: &cfg.Model, log: s.Logger}
			}
			return o, nil
		},
		component.WithFamily(family))
}
