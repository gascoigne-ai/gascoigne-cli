package execreceiver

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

const typ = "exec"

// NewFactory returns the factory that builds the "exec" receiver.
func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config { return &Config{Kind: payload.CmdOutput} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Receiver{cfg: cfg, next: set.Pipeline}, nil
		})
}
