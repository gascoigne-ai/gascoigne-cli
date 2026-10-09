package mcpreceiver

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "mcp"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config {
			return &Config{Ledger: "ledger", Retrieve: "retrieve", FetchTimeout: defaultTimeout, FetchMaxBytes: 5 << 20}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Receiver{
				cfg:       cfg,
				next:      set.Pipeline,
				log:       set.Logger,
				transport: &mcp.StdioTransport{},
				http:      &http.Client{Timeout: cfg.FetchTimeout},
			}, nil
		})
}
