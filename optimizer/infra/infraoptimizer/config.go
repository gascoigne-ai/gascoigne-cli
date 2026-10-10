package infraoptimizer

import "errors"

type Config struct {
	// MaxPlanLines caps plan resource bodies; past it, created and updated
	// resources keep their header only. Destroys and replacements stay whole.
	MaxPlanLines int `yaml:"max_plan_lines"`
	// DiffContext is the unchanged lines kept around helm diff changes.
	DiffContext int `yaml:"diff_context"`
}

func (c *Config) Validate() error {
	if c.MaxPlanLines < 20 || c.DiffContext < 0 {
		return errors.New("need max_plan_lines >= 20, diff_context >= 0")
	}
	return nil
}
