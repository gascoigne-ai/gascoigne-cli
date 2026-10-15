package dataoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

type Config struct {
	// JSON tunes the crusher YAML goes through.
	JSON jsonx.Config `yaml:"json"`
	// MaxRows and MaxCell tune CSV/TSV tables.
	MaxRows int `yaml:"max_rows"`
	MaxCell int `yaml:"max_cell"`
	// MaxItems caps repeated XML siblings.
	MaxItems int `yaml:"max_items"`
	// MaxOutputLines caps each text output of a notebook cell (head and
	// tail kept); error tracebacks stay whole.
	MaxOutputLines int `yaml:"max_output_lines"`
}

func (c *Config) Validate() error {
	if c.MaxRows < 5 || c.MaxCell < 10 || c.MaxItems < 3 || c.MaxOutputLines < 4 {
		return errors.New("need max_rows >= 5, max_cell >= 10, max_items >= 3, max_output_lines >= 4")
	}
	return c.JSON.Validate()
}
