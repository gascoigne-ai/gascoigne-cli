package mcpproxyreceiver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const typ = "mcp_proxy"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config { return &Config{Ledger: "ledger", Retrieve: "gascoigne_retrieve", CompactTools: true} },
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Receiver{cfg: cfg, next: set.Pipeline, log: set.Logger, upstream: &mcp.StdioTransport{}}, nil
		})
}
