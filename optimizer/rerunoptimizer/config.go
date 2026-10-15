package rerunoptimizer

import (
	"errors"
	"time"
)

// Config sets which re-runs are folded against what the agent saw before.
type Config struct {
	// Ledger is the extension keeping earlier outputs; "" disables folding.
	Ledger string `yaml:"ledger"`
	// Families lists the content families folded: command output agents
	// re-run in a loop (tests, builds), not files they edit by exact text.
	Families []string `yaml:"families"`
	// MaxAge is how far back earlier runs count: older ones may have left
	// the agent's context.
	MaxAge time.Duration `yaml:"max_age"`
	// Runs is how many earlier outputs are compared, so a run folded against
	// the one before still matches what that one showed in full.
	Runs int `yaml:"runs"`
	// MinLines and MinChars are the smallest run of lines worth a pointer.
	MinLines int `yaml:"min_lines"`
	MinChars int `yaml:"min_chars"`
}

func defaults() *Config {
	return &Config{
		Ledger:   "ledger",
		Families: []string{"test", "build", "task", "pkg", "log", "search", "vcs"},
		MaxAge:   20 * time.Minute,
		Runs:     3,
		MinLines: 4,
		MinChars: 200,
	}
}

func (c *Config) Validate() error {
	if c.Runs < 1 || c.MinLines < 2 {
		return errors.New("runs must be at least 1 and min_lines at least 2")
	}
	return nil
}
