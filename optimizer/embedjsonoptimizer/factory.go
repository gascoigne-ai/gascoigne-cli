package embedjsonoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

const (
	typ    = "embedjson"
	family = "core"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config {
			return &Config{Types: []string{"log", "test", "build", "task", "doc/text"}, JSON: *jsonx.Default()}
		},
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
