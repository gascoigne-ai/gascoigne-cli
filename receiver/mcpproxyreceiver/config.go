package mcpproxyreceiver

import "errors"

type Config struct {
	// Command starts the downstream MCP server (stdio). The CLI can pass it
	// instead: gascoigne mcp-proxy -- npx some-mcp-server
	Command []string `yaml:"command"`
	// Ledger is the extension id serving the retrieve tool.
	Ledger string `yaml:"ledger"`
	// Retrieve names the added tool returning originals; must not clash with downstream tools.
	Retrieve string `yaml:"retrieve"`
}

func (c *Config) Validate() error {
	if c.Retrieve == "" {
		return errors.New("retrieve must be set")
	}
	return nil
}
