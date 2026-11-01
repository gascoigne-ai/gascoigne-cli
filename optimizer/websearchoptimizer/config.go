package websearchoptimizer

import "errors"

// Config caps the links of one search.
type Config struct {
	// MaxLinks is the most links kept per Links: line; over it, the ones
	// matching the query best stay (in the engine's order).
	MaxLinks int `yaml:"max_links"`
}

func (c *Config) Validate() error {
	if c.MaxLinks < 1 {
		return errors.New("max_links must be at least 1")
	}
	return nil
}
