package httpproxyreceiver

import (
	"errors"
	"net/url"
)

type Config struct {
	// Anthropic and OpenAI are the upstream APIs requests are forwarded to.
	Anthropic string `yaml:"anthropic"`
	OpenAI    string `yaml:"openai"`
	// Skip lists tools whose results are never touched: edits confirm what
	// changed, plans and questions are for the user.
	Skip []string `yaml:"skip"`
	// CacheEntries bounds the tool results remembered by content hash, so
	// each is sent identically on every later request (prompt cache).
	CacheEntries int `yaml:"cache_entries"`
}

func (c *Config) Validate() error {
	for _, u := range []string{c.Anthropic, c.OpenAI} {
		if p, err := url.Parse(u); err != nil || p.Scheme == "" || p.Host == "" {
			return errors.New("anthropic and openai must be absolute URLs")
		}
	}
	if c.CacheEntries < 100 {
		return errors.New("cache_entries must be >= 100")
	}
	return nil
}
