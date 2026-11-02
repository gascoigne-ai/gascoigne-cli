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
	// ErrorContext is how many lines after each kept error stay (got/want).
	ErrorContext int `yaml:"error_context"`
	// QueryLines caps middle lines kept because they match the query attr.
	QueryLines int `yaml:"query_lines"`
	// QueryDiverse picks the query lines by maximal marginal relevance, so a
	// failure repeated under other ids does not use up all QueryLines.
	QueryDiverse bool `yaml:"query_diverse"`
	// Skip lists types or families never cut: source files are edited by
	// exact text and have their own outline; structured data would no longer
	// parse, and its engines already sample to their own budgets.
	Skip []string `yaml:"skip"`
}

func defaults() *Config {
	return &Config{MaxTokens: 10000, HeadTokens: 3200, TailTokens: 3200, MaxErrors: 20, ErrorContext: 2, QueryLines: 20, Skip: []string{"fs/code", "data", "k8s", "cloud"}}
}

func (c *Config) Validate() error {
	if c.HeadTokens < 0 || c.TailTokens < 0 || c.MaxErrors < 0 || c.ErrorContext < 0 || c.QueryLines < 0 {
		return errors.New("head_tokens, tail_tokens, max_errors, error_context and query_lines must not be negative")
	}
	if c.HeadTokens+c.TailTokens >= c.MaxTokens {
		return errors.New("head_tokens + tail_tokens must be below max_tokens")
	}
	return nil
}
