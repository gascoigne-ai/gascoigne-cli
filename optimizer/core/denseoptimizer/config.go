package denseoptimizer

import "errors"

// Config sets what counts as a dense line and how much of one stays.
type Config struct {
	// MinLine is the shortest line, in bytes, that can be dense.
	MinLine int `yaml:"min_line"`
	// MinTotal is the fewest dense bytes in a block worth eliding: a single
	// token or signed URL is a value the agent may need, a bundle is not.
	MinTotal int `yaml:"min_total"`
	// Head and Tail are bytes kept from the start and end of a dense line.
	Head int `yaml:"head"`
	Tail int `yaml:"tail"`
	// Skip lists types or families never touched: structured data has its
	// own engines. Source read from disk is always left alone.
	Skip []string `yaml:"skip"`
}

func defaults() *Config {
	return &Config{MinLine: 300, MinTotal: 2000, Head: 160, Tail: 80, Skip: []string{"data", "k8s", "cloud"}}
}

func (c *Config) Validate() error {
	if c.Head < 0 || c.Tail < 0 {
		return errors.New("head and tail must not be negative")
	}
	if c.Head+c.Tail >= c.MinLine {
		return errors.New("head + tail must be below min_line")
	}
	return nil
}
