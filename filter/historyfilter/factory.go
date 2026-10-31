package historyfilter

import (
	"context"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const typ = "history"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindFilter, typ,
		func() *Config {
			return &Config{Ledger: "ledger", MaxFailures: 2, FetchMaxFailures: 3, FailureTTL: 24 * time.Hour}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Filter{cfg: cfg, log: set.Logger}, nil
		})
}
