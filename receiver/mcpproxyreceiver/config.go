package mcpproxyreceiver

import (
	"errors"
	"net/url"
)

type Config struct {
	// Command starts the downstream MCP server (stdio). The CLI can pass it
	// instead: gascoigne mcp-proxy -- npx some-mcp-server
	Command []string `yaml:"command"`
	// URL, instead of Command, is a remote MCP server (streamable HTTP, or SSE
	// when SSE is set); Headers are sent with every request, e.g. Authorization.
	URL     string            `yaml:"url"`
	SSE     bool              `yaml:"sse"`
	Headers map[string]string `yaml:"headers"`
	// Ledger is the extension id serving the retrieve tool.
	Ledger string `yaml:"ledger"`
	// Retrieve names the added tool returning originals; must not clash with downstream tools.
	Retrieve string `yaml:"retrieve"`
	// CompactTools drops what downstream tool schemas say twice (titles
	// restating names, $schema markers); constraints stay.
	CompactTools bool `yaml:"compact_tools"`
	// MaxTools hides the downstream tools behind search_tools and call_tool
	// when there are more than this many, so agents without tool search do
	// not send every schema with every request; 0 always mirrors them.
	MaxTools int `yaml:"max_tools"`
}

func (c *Config) Validate() error {
	if c.MaxTools < 0 {
		return errors.New("max_tools must not be negative")
	}
	if c.URL != "" {
		if u, err := url.Parse(c.URL); err != nil || u.Host == "" || u.Scheme != "http" && u.Scheme != "https" {
			return errors.New("url must be an http or https URL")
		}
		if len(c.Command) > 0 {
			return errors.New("set url or command, not both")
		}
	}
	if c.Retrieve == "" {
		return errors.New("retrieve must be set")
	}
	return nil
}
