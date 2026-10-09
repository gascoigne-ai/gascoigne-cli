package detectfilter

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "detect"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindFilter, typ,
		func() *Config { return &Config{} },
		func(_ context.Context, set component.Settings, _ *Config) (component.Component, error) {
			return &Filter{log: set.Logger}, nil
		})
}
