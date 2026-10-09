package jsonoptimizer

import "errors"

type Config struct {
	// DropEmpty removes null, "", [] and {} values.
	DropEmpty bool `yaml:"drop_empty"`
	// MaxItems samples longer arrays: ends, errors, outliers and rare values survive.
	MaxItems int `yaml:"max_items"`
	// KeepEnds is how many first and last items a sampled array keeps.
	KeepEnds int `yaml:"keep_ends"`
	// TabulateMin turns arrays (and maps) of at least this many similar objects into tables.
	TabulateMin int `yaml:"tabulate_min"`
	// MaxString truncates longer strings in the middle.
	MaxString int `yaml:"max_string"`
	// ShortenHashes cuts long hash/base64 tokens (checksums, ids) to a prefix.
	ShortenHashes bool `yaml:"shorten_hashes"`
}

func (c *Config) Validate() error {
	if c.MaxItems < 2*c.KeepEnds+1 || c.KeepEnds < 1 || c.TabulateMin < 2 || c.MaxString < 20 {
		return errors.New("need keep_ends >= 1, max_items > 2*keep_ends, tabulate_min >= 2, max_string >= 20")
	}
	return nil
}
