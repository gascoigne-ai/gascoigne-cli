package buildoptimizer

import "errors"

type Config struct {
	// MaxFrame caps code frame lines kept per diagnostic (dialects may lower it).
	MaxFrame int `yaml:"max_frame"`
}

func (c *Config) Validate() error {
	if c.MaxFrame < 0 {
		return errors.New("max_frame must be >= 0")
	}
	return nil
}
