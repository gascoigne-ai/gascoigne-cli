package gitoptimizer

import "errors"

type Config struct {
	// ContextLines kept around changes in diffs.
	ContextLines int `yaml:"context_lines"`
	// MaxFileLines caps diff lines per file; the rest becomes a marker.
	MaxFileLines int `yaml:"max_file_lines"`
}

func (c *Config) Validate() error {
	if c.ContextLines < 0 || c.MaxFileLines < 1 {
		return errors.New("context_lines must be >= 0 and max_file_lines >= 1")
	}
	return nil
}
