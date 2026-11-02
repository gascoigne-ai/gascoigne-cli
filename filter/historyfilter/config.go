package historyfilter

import (
	"errors"
	"time"
)

type Config struct {
	// MaxFailures bypasses a key once it failed this many times.
	MaxFailures int `yaml:"max_failures"`
	// FetchMaxFailures is MaxFailures for fetch URLs read whole on purpose
	// (a doc page): a full retrieve there is a weaker sign of a bad result.
	FetchMaxFailures int `yaml:"fetch_max_failures"`
	// GentleAfter routes a key that failed this many times, but not yet
	// MaxFailures (a key that reaches it is bypassed instead), through the gentle
	// route (attr level=gentle); 0 turns it off.
	GentleAfter int `yaml:"gentle_after"`
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
	if c.GentleAfter < 0 {
		return errors.New("gentle_after must not be negative")
	}
	if c.FailureTTL < 0 {
		return errors.New("failure_ttl must not be negative")
	}
	return nil
}
