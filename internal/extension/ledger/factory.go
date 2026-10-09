package ledger

import (
	"context"

	"github.com/lukaszb/gascoigne/internal/component"
)

const typ = "ledger"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindExtension, typ,
		func() *Config { return &Config{RawTTL: DefaultRawTTL} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return New(cfg), nil
		})
}
