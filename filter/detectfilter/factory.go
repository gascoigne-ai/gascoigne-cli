package detectfilter

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "detect"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindFilter, typ,
		func() *Config {
			return &Config{EditSafe: []string{"data", "k8s", "cloud", "infra"}, EditSafeMaxTokens: 4000}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Filter{cfg: cfg, log: set.Logger}, nil
		})
}
