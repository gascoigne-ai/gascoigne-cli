package grepoptimizer

import "errors"

type Config struct {
	// MaxPerFile caps matches shown per file.
	MaxPerFile int `yaml:"max_per_file"`
	// MaxFiles caps files shown.
	MaxFiles int `yaml:"max_files"`
	// MaxLineLen truncates long matched lines (minified code, data).
	MaxLineLen int `yaml:"max_line_len"`
}

func (c *Config) Validate() error {
	if c.MaxPerFile < 1 || c.MaxFiles < 1 || c.MaxLineLen < 20 {
		return errors.New("max_per_file and max_files must be >= 1, max_line_len >= 20")
	}
	return nil
}
