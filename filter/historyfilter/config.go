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
	// FamilyWindow, FamilyMinRuns and FamilyRerequestRate send every source of
	// an optimizer family through the gentle route when, in the window, at
	// least that share of its runs (of at least that many) was fetched raw
	// afterwards. A rate of 0 turns it off.
	FamilyWindow        time.Duration `yaml:"family_window"`
	FamilyMinRuns       int           `yaml:"family_min_runs"`
	FamilyRerequestRate float64       `yaml:"family_rerequest_rate"`
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
	if c.FamilyRerequestRate < 0 || c.FamilyRerequestRate > 1 || c.FamilyMinRuns < 1 && c.FamilyRerequestRate > 0 || c.FamilyWindow <= 0 && c.FamilyRerequestRate > 0 {
		return errors.New("family_rerequest_rate must be in [0,1] and, above 0, family_min_runs at least 1 and family_window positive")
	}
	if c.FailureTTL < 0 {
		return errors.New("failure_ttl must not be negative")
	}
	return nil
}
