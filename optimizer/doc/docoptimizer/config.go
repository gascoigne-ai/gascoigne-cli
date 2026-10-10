package docoptimizer

import "errors"

type Config struct {
	// MaxCodeLines keeps head and tail of longer code blocks.
	MaxCodeLines int `yaml:"max_code_lines"`
	// ManDrop lists man page sections dropped as boilerplate.
	ManDrop []string `yaml:"man_drop"`
}

func (c *Config) Validate() error {
	if c.MaxCodeLines < 6 {
		return errors.New("need max_code_lines >= 6")
	}
	return nil
}
