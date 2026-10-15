package gitoptimizer

import "errors"

type Config struct {
	// ContextLines kept around changes in diffs.
	ContextLines int `yaml:"context_lines"`
	// MaxFileLines caps diff lines per file; the rest becomes a marker.
	MaxFileLines int `yaml:"max_file_lines"`
	// MinMoveLines is the shortest removed block shown as moved when it
	// comes back verbatim elsewhere in the diff; 0 turns it off.
	MinMoveLines int `yaml:"min_move_lines"`
}

func (c *Config) Validate() error {
	if c.ContextLines < 0 || c.MaxFileLines < 1 || c.MinMoveLines < 0 {
		return errors.New("context_lines and min_move_lines must be >= 0, max_file_lines >= 1")
	}
	return nil
}
