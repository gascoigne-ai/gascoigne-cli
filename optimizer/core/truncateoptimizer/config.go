package truncateoptimizer

import "errors"

type Config struct {
	// MaxTokens is the largest block left whole after every other optimizer.
	MaxTokens int `yaml:"max_tokens"`
	// HeadTokens and TailTokens are kept from the start and the end.
	HeadTokens int `yaml:"head_tokens"`
	TailTokens int `yaml:"tail_tokens"`
	// MaxErrors caps error lines kept from the cut middle.
	MaxErrors int `yaml:"max_errors"`
	// Skip lists types or families never cut: source files are edited by
	// exact text and have their own outline; structured data would no longer
	// parse, and its engines already sample to their own budgets.
	Skip []string `yaml:"skip"`
}

func defaults() *Config {
	return &Config{MaxTokens: 8000, HeadTokens: 2500, TailTokens: 2500, MaxErrors: 20, Skip: []string{"fs/code", "data", "k8s", "cloud"}}
}

func (c *Config) Validate() error {
	if c.HeadTokens < 0 || c.TailTokens < 0 || c.MaxErrors < 0 {
		return errors.New("head_tokens, tail_tokens and max_errors must not be negative")
	}
	if c.HeadTokens+c.TailTokens >= c.MaxTokens {
		return errors.New("head_tokens + tail_tokens must be below max_tokens")
	}
	return nil
}
