package proseoptimizer

import "errors"

type Config struct {
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
	if c.Enabled && (c.Threshold <= 0 || c.Threshold >= 1 || c.MinWords < 1) {
		return errors.New("need 0 < threshold < 1 and min_words >= 1")
	}
	return nil
}
