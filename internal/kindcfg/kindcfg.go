// Package kindcfg is the config of receivers that wrap what they read in one
// payload of a configured kind (exec, hook, stdin).
package kindcfg

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Config is the receiver config that holds only the kind of the payload.
type Config struct {
	// Kind of the payload the receiver emits.
	Kind payload.Kind `yaml:"kind"`
}

// Validate requires a kind.
func (c *Config) Validate() error {
	if c.Kind == "" {
		return errors.New("kind must be set")
	}
	return nil
}
