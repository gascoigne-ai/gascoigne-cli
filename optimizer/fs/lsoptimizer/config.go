package lsoptimizer

import "errors"

type Config struct {
	// SizesUpTo shows file sizes when a listing has at most this many files.
	SizesUpTo int `yaml:"sizes_up_to"`
	// Width wraps entry lines at this many characters.
	Width int `yaml:"width"`
	// MaxPerDir lists this many files per directory in trees and path
	// lists; du keeps twice as many entries.
	MaxPerDir int `yaml:"max_per_dir"`
}

func (c *Config) Validate() error {
	if c.SizesUpTo < 0 || c.Width < 20 || c.MaxPerDir < 3 {
		return errors.New("sizes_up_to must be >= 0, width >= 20, max_per_dir >= 3")
	}
	return nil
}
