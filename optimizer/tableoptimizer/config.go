package tableoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

type Config struct {
	// Limits sample longer tables (max_rows: unhealthy rows, anomalies, query
	// matches and ends stay) and cut wide cells (max_cell).
	table.Limits `yaml:",inline"`
}

func (c *Config) Validate() error {
	if !c.Valid() {
		return errors.New("need max_rows >= 5 and max_cell >= 10")
	}
	return nil
}
