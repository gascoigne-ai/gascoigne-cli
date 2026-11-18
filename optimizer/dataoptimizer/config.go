package dataoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

// Config tunes the data engine: the JSON crusher that YAML goes through, the table
// limits and the caps for XML siblings and notebook cell outputs.
type Config struct {
	// JSON tunes the crusher YAML goes through.
	JSON jsonx.Config `yaml:"json"`
	// Limits tune CSV/TSV tables.
	table.Limits `yaml:",inline"`
	// MaxItems caps repeated XML siblings.
	MaxItems int `yaml:"max_items"`
	// MaxOutputLines caps each text output of a notebook cell (head and
	// tail kept); error tracebacks stay whole.
	MaxOutputLines int `yaml:"max_output_lines"`
}

func (c *Config) Validate() error {
	if !c.Valid() || c.MaxItems < 3 || c.MaxOutputLines < 4 {
		return errors.New("need max_rows >= 5, max_cell >= 10, max_items >= 3, max_output_lines >= 4")
	}
	return c.JSON.Validate()
}
