package httpproxyreceiver

import (
	"errors"
	"net/url"
)

type Config struct {
	// Anthropic, OpenAI and Gemini are the upstream APIs requests are forwarded to.
	Anthropic string `yaml:"anthropic"`
	OpenAI    string `yaml:"openai"`
	Gemini    string `yaml:"gemini"`
	// Skip lists tools whose results are never touched: edits confirm what
	// changed, plans and questions are for the user.
	Skip []string `yaml:"skip"`
	// CacheEntries bounds the tool results remembered by content hash, so
	// each is sent identically on every later request (prompt cache).
	CacheEntries int `yaml:"cache_entries"`
	// Ledger is the extension keeping originals of folded results; folding
	// needs it.
	Ledger string `yaml:"ledger"`
	// FoldMinLines is the shortest run of a result, already shown by an
	// earlier result of the request, replaced by a pointer; 0 turns it off.
	FoldMinLines int `yaml:"fold_min_lines"`
	// FoldMinChars is the fewest bytes such a run must have.
	FoldMinChars int `yaml:"fold_min_chars"`
}

func (c *Config) Validate() error {
	for _, u := range []string{c.Anthropic, c.OpenAI, c.Gemini} {
		if p, err := url.Parse(u); err != nil || p.Scheme == "" || p.Host == "" {
			return errors.New("anthropic, openai and gemini must be absolute URLs")
		}
	}
	if c.FoldMinLines < 0 || c.FoldMinChars < 0 {
		return errors.New("fold_min_lines and fold_min_chars must not be negative")
	}
	if c.CacheEntries < 100 {
		return errors.New("cache_entries must be >= 100")
	}
	return nil
}
