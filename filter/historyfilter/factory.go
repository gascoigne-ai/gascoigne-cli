package historyfilter

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "history"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindFilter, typ,
		func() *Config { return &Config{Ledger: "ledger", MaxFailures: 1} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Filter{cfg: cfg, log: set.Logger}, nil
		})
}
