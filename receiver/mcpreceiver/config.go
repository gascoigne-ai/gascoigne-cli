package mcpreceiver

import (
	"errors"
	"time"
)

const defaultTimeout = 20 * time.Second

type Config struct {
	// Ledger is the extension id serving retrieve and stats.
	Ledger string `yaml:"ledger"`
	// Retrieve names the tool returning originals.
	Retrieve string `yaml:"retrieve"`
	// FetchTimeout bounds one fetch.
	FetchTimeout time.Duration `yaml:"fetch_timeout"`
	// FetchMaxBytes caps how much of a response body is read.
	FetchMaxBytes int64 `yaml:"fetch_max_bytes"`
}

func (c *Config) Validate() error {
	if c.Retrieve == "" || c.FetchTimeout <= 0 || c.FetchMaxBytes <= 0 {
		return errors.New("retrieve, fetch_timeout and fetch_max_bytes must be set")
	}
	return nil
}
