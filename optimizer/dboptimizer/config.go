package dboptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

type Config struct {
	// Limits sample longer result sets and document lists (max_rows) and cut
	// wide cells (max_cell).
	table.Limits `yaml:",inline"`
	// PlanMinShare keeps EXPLAIN nodes taking at least this share of the
	// slowest node's time; cheaper ones collapse.
	PlanMinShare float64 `yaml:"plan_min_share"`
}

func (c *Config) Validate() error {
	if !c.Valid() || c.PlanMinShare < 0 || c.PlanMinShare >= 1 {
		return errors.New("need max_rows >= 5, max_cell >= 10, 0 <= plan_min_share < 1")
	}
	return nil
}
