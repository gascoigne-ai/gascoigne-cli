package logoptimizer

import "errors"

type Config struct {
	// Context lines kept around warnings and errors.
	Context int `yaml:"context"`
	// DropDebug removes DEBUG and TRACE lines.
	DropDebug bool `yaml:"drop_debug"`
	// MaxLines caps the result; warnings and errors are kept first.
	MaxLines int `yaml:"max_lines"`
}

func (c *Config) Validate() error {
	if c.Context < 0 || c.MaxLines < 20 {
		return errors.New("context must be >= 0 and max_lines >= 20")
	}
	return nil
}
