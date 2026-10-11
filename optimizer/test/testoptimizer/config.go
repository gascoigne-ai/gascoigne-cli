package testoptimizer

import "errors"

type Config struct {
	// CoverageBelow is the coverage percent under which a file of a
	// coverage report is listed; the others are counted.
	CoverageBelow float64 `yaml:"coverage_below"`
}

func (c *Config) Validate() error {
	if c.CoverageBelow < 0 || c.CoverageBelow > 100 {
		return errors.New("coverage_below must be a percent, 0 to 100")
	}
	return nil
}
