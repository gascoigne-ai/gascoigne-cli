package bypassfilter

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const typ = "bypass"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindFilter, typ,
		func() *Config { return &Config{Env: DefaultEnv} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Filter{cfg: cfg}, nil
		})
}
