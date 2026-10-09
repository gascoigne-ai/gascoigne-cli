package historyfilter

import (
	"errors"

	"github.com/lukaszb/gascoigne/internal/component"
)

type Config struct {
	// Ledger is the extension id holding failure history.
	Ledger string `yaml:"ledger"`
	// MaxFailures bypasses a key once it failed this many times.
	MaxFailures int `yaml:"max_failures"`
}

func (c *Config) Validate() error {
	if c.MaxFailures < 1 {
		return errors.New("max_failures must be at least 1")
	}
	_, err := component.ParseID(c.Ledger)
	return err
}
