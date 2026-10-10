package hookreceiver

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

const typ = "hook"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config { return &Config{Kind: payload.CmdOutput} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Receiver{cfg: cfg, next: set.Pipeline}, nil
		})
}
