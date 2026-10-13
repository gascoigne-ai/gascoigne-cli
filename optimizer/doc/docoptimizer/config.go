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
	// Model compresses prose paragraphs with a local model (opt-in, lossy:
	// the original stays in the ledger).
	Model Model `yaml:"model"`
}

// Model configures the prose model process (package prose).
type Model struct {
	Enabled bool `yaml:"enabled"`
	// Command starts the model process: program and arguments (a list, so
	// Windows paths may hold spaces). Empty runs this binary's `prose serve`.
	Command []string `yaml:"command"`
	// Threshold is the keep score from which a word stays; higher drops more.
	Threshold float64 `yaml:"threshold"`
	// Warm loads the model when the optimizer starts, so the server's first
	// request does not wait for it. A run without a server pays for the load
	// in the background too.
	Warm bool `yaml:"warm"`
	// MinWords skips shorter paragraphs: dropping a word or two saves nothing.
	MinWords int `yaml:"min_words"`
}

func (c *Config) Validate() error {
	if c.MaxCodeLines < 6 || c.MaxTokens < 500 {
		return errors.New("need max_code_lines >= 6, max_tokens >= 500")
	}
	if k := c.Model; k.Enabled && (k.Threshold <= 0 || k.Threshold >= 1 || k.MinWords < 1) {
		return errors.New("model needs 0 < threshold < 1 and min_words >= 1")
	}
	return nil
}
