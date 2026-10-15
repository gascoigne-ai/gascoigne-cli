package docoptimizer

import "errors"

type Config struct {
	// MaxCodeLines keeps head and tail of longer code blocks.
	MaxCodeLines int `yaml:"max_code_lines"`
	// MaxTokens fits longer markdown (fetched pages, READMEs) into this budget
	// by whole sections, those matching the query attr first.
	MaxTokens int `yaml:"max_tokens"`
	// ManDrop lists man page sections dropped as boilerplate.
	ManDrop []string `yaml:"man_drop"`
}

func (c *Config) Validate() error {
	if c.MaxCodeLines < 6 || c.MaxTokens < 500 {
		return errors.New("need max_code_lines >= 6, max_tokens >= 500")
	}
	return nil
}
