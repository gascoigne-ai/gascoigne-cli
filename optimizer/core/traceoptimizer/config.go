package traceoptimizer

import "errors"

// Config sets which output has its stack traces folded and how.
type Config struct {
	// Families lists the content families searched for traces; doc covers
	// unrecognized text (doc/text).
	Families []string `yaml:"families"`
	// MaxPeriod is the longest frame cycle, in lines, a recursion is
	// searched for.
	MaxPeriod int `yaml:"max_period"`
	// MinRepeats is how often a cycle must occur in a row to be folded.
	MinRepeats int `yaml:"min_repeats"`
	// MinGoroutines is the smallest group of goroutines on one stack shown
	// once.
	MinGoroutines int `yaml:"min_goroutines"`
}

func (c *Config) Validate() error {
	if c.MaxPeriod < 1 || c.MinRepeats < 2 || c.MinGoroutines < 2 {
		return errors.New("max_period must be at least 1, min_repeats and min_goroutines at least 2")
	}
	return nil
}
