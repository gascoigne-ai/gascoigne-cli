package ariaoptimizer

import "errors"

// Config caps runs of like siblings (menu links, list items, table rows).
type Config struct {
	// MaxItems is the longest run of siblings with the same role kept whole.
	MaxItems int `yaml:"max_items"`
	// Head and Tail are the siblings kept from the start and end of a longer
	// run; ones matching the query attr are kept too.
	Head int `yaml:"head"`
	Tail int `yaml:"tail"`
}

func (c *Config) Validate() error {
	if c.Head < 0 || c.Tail < 0 || c.Head+c.Tail >= c.MaxItems {
		return errors.New("head and tail must not be negative and head + tail must be below max_items")
	}
	return nil
}
