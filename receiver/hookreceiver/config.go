package hookreceiver

import (
	"errors"

	"github.com/lukaszb/gascoigne/payload"
)

type Config struct {
	Kind payload.Kind `yaml:"kind"`
}

func (c *Config) Validate() error {
	if c.Kind == "" {
		return errors.New("kind must be set")
	}
	return nil
}
