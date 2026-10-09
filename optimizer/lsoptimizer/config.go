package lsoptimizer

import "errors"

type Config struct {
	// SizesUpTo shows file sizes when a listing has at most this many files.
	SizesUpTo int `yaml:"sizes_up_to"`
	// Width wraps entry lines at this many characters.
	Width int `yaml:"width"`
}

func (c *Config) Validate() error {
	if c.SizesUpTo < 0 || c.Width < 20 {
		return errors.New("sizes_up_to must be >= 0 and width >= 20")
	}
	return nil
}
