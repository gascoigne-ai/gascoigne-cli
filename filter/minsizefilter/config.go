package minsizefilter

import "errors"

type Config struct {
	MinTokens int `yaml:"min_tokens"`
}

func (c *Config) Validate() error {
	if c.MinTokens < 0 {
		return errors.New("min_tokens must not be negative")
	}
	return nil
}
