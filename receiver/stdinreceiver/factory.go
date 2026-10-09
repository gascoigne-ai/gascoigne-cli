package stdinreceiver

import (
	"context"
	"os"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

const typ = "stdin"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config { return &Config{Kind: payload.CmdOutput} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Receiver{cfg: cfg, next: set.Pipeline, in: os.Stdin}, nil
		})
}
