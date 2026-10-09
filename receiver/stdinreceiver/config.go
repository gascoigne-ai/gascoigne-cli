package stdinreceiver

import (
	"errors"

	"github.com/lukaszb/gascoigne/payload"
)

type Config struct {
	// Kind of the single block read from stdin.
	Kind payload.Kind `yaml:"kind"`
}

func (c *Config) Validate() error {
	if c.Kind == "" {
		return errors.New("kind must be set")
	}
	return nil
}
