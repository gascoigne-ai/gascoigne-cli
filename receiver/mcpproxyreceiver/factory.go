package mcpproxyreceiver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const typ = "mcp_proxy"

// NewFactory returns the factory that builds the "mcp_proxy" receiver.
func NewFactory(opts ...Option) component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config { return &Config{Retrieve: "gascoigne_retrieve", CompactTools: true} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			r := &Receiver{cfg: cfg, next: set.Pipeline, log: set.Logger, upstream: &mcp.StdioTransport{}}
			for _, opt := range opts {
				opt(r)
			}
			return r, nil
		})
}
