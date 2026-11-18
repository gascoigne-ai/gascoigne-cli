package httpoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

// Config tunes the HTTP response engine: the headers to drop, the JSON crusher for
// bodies and whether HTML links are kept.
type Config struct {
	// DropHeaders are response headers (lowercase) that tell an agent nothing.
	DropHeaders []string `yaml:"drop_headers"`
	// JSON tunes the crusher for JSON bodies.
	JSON jsonx.Config `yaml:"json"`
	// Links keeps hrefs when HTML bodies become markdown.
	Links bool `yaml:"links"`
}

func (c *Config) Validate() error {
	if len(c.DropHeaders) == 0 {
		return errors.New("drop_headers must not be empty")
	}
	return c.JSON.Validate()
}
