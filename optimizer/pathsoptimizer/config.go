package pathsoptimizer

import (
	"errors"
	"slices"
)

type Config struct {
	// Families whose blocks get aliases. Source files, data and listings are
	// left alone: agents copy those verbatim.
	Families []string `yaml:"families"`
	// MinPrefix is the shortest path prefix worth an alias, in bytes.
	MinPrefix int `yaml:"min_prefix"`
	// MinSaving is the fewest tokens all aliases together must save, legend included.
	MinSaving int `yaml:"min_saving"`
	// MaxAliases caps the legend.
	MaxAliases int `yaml:"max_aliases"`
}

func defaults() *Config {
	return &Config{Families: []string{"test", "build", "search", "log", "task"}, MinPrefix: 20, MinSaving: 30, MaxAliases: 9}
}

func (c *Config) Validate() error {
	if c.MinPrefix < 8 {
		return errors.New("min_prefix must be at least 8")
	}
	if c.MaxAliases < 1 || c.MaxAliases > 99 {
		return errors.New("max_aliases must be 1-99")
	}
	if slices.Contains(c.Families, "fs") {
		return errors.New("families must not include fs: source files are edited by exact text")
	}
	return nil
}
