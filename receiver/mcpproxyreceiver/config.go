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
	// CompactTools drops what downstream tool schemas say twice (titles
	// restating names, $schema markers); constraints stay.
	CompactTools bool `yaml:"compact_tools"`
}

func (c *Config) Validate() error {
	if c.Retrieve == "" {
		return errors.New("retrieve must be set")
	}
	return nil
}
