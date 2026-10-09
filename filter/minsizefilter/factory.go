package minsizefilter

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "minsize"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindFilter, typ,
		func() *Config { return &Config{MinTokens: 200} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Filter{cfg: cfg}, nil
		})
}
