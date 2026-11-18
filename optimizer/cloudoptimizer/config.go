package cloudoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

type Config struct {
	// JSON tunes the JSON crusher used for API responses.
	JSON jsonx.Config `yaml:"json"`
	// Limits tune tables (-o table, gcloud list).
	table.Limits `yaml:",inline"`
	// MaxObjects lists object listings (aws s3 ls) in full up to this size.
	MaxObjects int `yaml:"max_objects"`
}

func (c *Config) Validate() error {
	if !c.Valid() || c.MaxObjects < 5 {
		return errors.New("need max_rows >= 5, max_cell >= 10, max_objects >= 5")
	}
	return c.JSON.Validate()
}
