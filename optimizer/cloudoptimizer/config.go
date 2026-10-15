package cloudoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

type Config struct {
	// JSON tunes the JSON crusher used for API responses.
	JSON jsonx.Config `yaml:"json"`
	// MaxRows and MaxCell tune tables (-o table, gcloud list).
	MaxRows int `yaml:"max_rows"`
	MaxCell int `yaml:"max_cell"`
	// MaxObjects lists object listings (aws s3 ls) in full up to this size.
	MaxObjects int `yaml:"max_objects"`
}

func (c *Config) Validate() error {
	if c.MaxRows < 5 || c.MaxCell < 10 || c.MaxObjects < 5 {
		return errors.New("need max_rows >= 5, max_cell >= 10, max_objects >= 5")
	}
	return c.JSON.Validate()
}
