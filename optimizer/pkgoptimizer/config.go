package pkgoptimizer

import "errors"

type Config struct {
	// TreeDepth is the deepest package of a dependency tree that gets its own
	// line; below it only a count stays. Packages leading to a query term are
	// never cut.
	TreeDepth int `yaml:"tree_depth"`
}

func (c *Config) Validate() error {
	if c.TreeDepth < 1 {
		return errors.New("tree_depth must be >= 1")
	}
	return nil
}
