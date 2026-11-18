package k8soptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

type Config struct {
	// MaxItems samples longer resource lists (kind: List); unhealthy items stay.
	MaxItems int `yaml:"max_items"`
	// Limits tune tables (kubectl get, events, helm list).
	table.Limits `yaml:",inline"`
	// NotesLines caps helm release NOTES.
	NotesLines int `yaml:"notes_lines"`
}

func (c *Config) Validate() error {
	if c.MaxItems < 2 || !c.Valid() || c.NotesLines < 0 {
		return errors.New("need max_items >= 2, max_rows >= 5, max_cell >= 10, notes_lines >= 0")
	}
	return nil
}
