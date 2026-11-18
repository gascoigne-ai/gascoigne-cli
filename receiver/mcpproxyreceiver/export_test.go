package mcpproxyreceiver

import "github.com/modelcontextprotocol/go-sdk/mcp"

// WithTransport sets the downstream transport directly (tests, in-process servers).
func WithTransport(transport mcp.Transport) Option {
	return func(rec *Receiver) { rec.transport = transport }
}
