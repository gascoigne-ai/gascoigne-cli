package tableoptimizer

import "errors"

type Config struct {
	// MaxRows samples longer tables: unhealthy rows, anomalies, query matches and ends stay.
	MaxRows int `yaml:"max_rows"`
	// MaxCell cuts longer cells in the middle.
	MaxCell int `yaml:"max_cell"`
}

func (c *Config) Validate() error {
	if c.MaxRows < 5 || c.MaxCell < 10 {
		return errors.New("need max_rows >= 5 and max_cell >= 10")
	}
	return nil
}
