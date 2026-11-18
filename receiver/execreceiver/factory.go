package execreceiver

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

const typ = "exec"

// NewFactory returns the factory that builds the "exec" receiver.
func NewFactory(opts ...Option) component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config { return &Config{Kind: payload.CmdOutput} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			r := &Receiver{cfg: cfg, next: set.Pipeline}
			for _, opt := range opts {
				opt(r)
			}
			return r, nil
		})
}
