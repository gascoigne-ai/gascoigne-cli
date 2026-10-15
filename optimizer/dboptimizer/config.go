package dboptimizer

import "errors"

type Config struct {
	// MaxRows samples longer result sets (and document lists).
	MaxRows int `yaml:"max_rows"`
	// MaxCell cuts longer cells in the middle.
	MaxCell int `yaml:"max_cell"`
	// PlanMinShare keeps EXPLAIN nodes taking at least this share of the
	// slowest node's time; cheaper ones collapse.
	PlanMinShare float64 `yaml:"plan_min_share"`
}

func (c *Config) Validate() error {
	if c.MaxRows < 5 || c.MaxCell < 10 || c.PlanMinShare < 0 || c.PlanMinShare >= 1 {
		return errors.New("need max_rows >= 5, max_cell >= 10, 0 <= plan_min_share < 1")
	}
	return nil
}
