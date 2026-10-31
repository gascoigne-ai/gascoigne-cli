package historyfilter

import (
	"errors"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

type Config struct {
	// Ledger is the extension id holding failure history.
	Ledger string `yaml:"ledger"`
	// MaxFailures bypasses a key once it failed this many times.
	MaxFailures int `yaml:"max_failures"`
	// FetchMaxFailures is MaxFailures for fetch URLs read whole on purpose
	// (a doc page): a full retrieve there is a weaker sign of a bad result.
	FetchMaxFailures int `yaml:"fetch_max_failures"`
	// FailureTTL ignores a key whose last failure is older; 0 never expires.
	FailureTTL time.Duration `yaml:"failure_ttl"`
}

func (c *Config) Validate() error {
	if c.MaxFailures < 1 {
		return errors.New("max_failures must be at least 1")
	}
	if c.FetchMaxFailures < c.MaxFailures {
		return errors.New("fetch_max_failures must be at least max_failures")
	}
	if c.FailureTTL < 0 {
		return errors.New("failure_ttl must not be negative")
	}
	_, err := component.ParseID(c.Ledger)
	return err
}
