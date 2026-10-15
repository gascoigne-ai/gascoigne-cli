package k8soptimizer

import "errors"

type Config struct {
	// MaxItems samples longer resource lists (kind: List); unhealthy items stay.
	MaxItems int `yaml:"max_items"`
	// MaxRows and MaxCell tune tables (kubectl get, events, helm list).
	MaxRows int `yaml:"max_rows"`
	MaxCell int `yaml:"max_cell"`
	// NotesLines caps helm release NOTES.
	NotesLines int `yaml:"notes_lines"`
}

func (c *Config) Validate() error {
	if c.MaxItems < 2 || c.MaxRows < 5 || c.MaxCell < 10 || c.NotesLines < 0 {
		return errors.New("need max_items >= 2, max_rows >= 5, max_cell >= 10, notes_lines >= 0")
	}
	return nil
}
