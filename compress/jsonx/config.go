package jsonx

import "errors"

type Config struct {
	// DropEmpty removes null, "", [] and {} values.
	DropEmpty bool `yaml:"drop_empty"`
	// MaxString truncates longer strings in the middle.
	MaxString int `yaml:"max_string"`
	// ShortenHashes cuts long hash/base64 tokens (checksums, ids) to a prefix.
	ShortenHashes bool `yaml:"shorten_hashes"`
	// TabulateMin is the fewest similar objects worth a table.
	TabulateMin int `yaml:"tabulate_min"`

	// Arrays go lossless first: a table keeping every item, accepted when it
	// saves at least LosslessMinSaving and stays under LosslessMaxTokens.
	LosslessMinSaving float64 `yaml:"lossless_min_saving"`
	LosslessMaxTokens int     `yaml:"lossless_max_tokens"`

	// Otherwise arrays of at least MinItems items and MinTokens tokens are
	// sampled down to about MaxItems: errors always stay, then anomalies,
	// change points and query matches, then head/tail shares, then an even
	// sample of the rest. Identical items collapse.
	MinItems   int     `yaml:"min_items"`
	MinTokens  int     `yaml:"min_tokens"`
	MaxItems   int     `yaml:"max_items"`
	HeadShare  float64 `yaml:"head_share"`
	TailShare  float64 `yaml:"tail_share"`
	AnomalyStd float64 `yaml:"anomaly_std"`
	// RelevanceK keeps the best matches for the payload's query attr.
	RelevanceK int `yaml:"relevance_k"`
}

// Default is the recommended configuration.
func Default() *Config {
	return &Config{
		DropEmpty: true, MaxString: 300, ShortenHashes: true, TabulateMin: 3,
		LosslessMinSaving: 0.3, LosslessMaxTokens: 1500,
		MinItems: 5, MinTokens: 200, MaxItems: 15, HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2, RelevanceK: 3,
	}
}

func (c *Config) Validate() error {
	switch {
	case c.MaxString < 20, c.TabulateMin < 2:
		return errors.New("need max_string >= 20 and tabulate_min >= 2")
	case c.LosslessMinSaving < 0 || c.LosslessMinSaving >= 1 || c.LosslessMaxTokens < 0:
		return errors.New("need 0 <= lossless_min_saving < 1 and lossless_max_tokens >= 0")
	case c.MinItems < 2 || c.MaxItems < 2 || c.MinTokens < 0:
		return errors.New("need min_items >= 2, max_items >= 2, min_tokens >= 0")
	case c.HeadShare < 0 || c.TailShare < 0 || c.HeadShare+c.TailShare > 1:
		return errors.New("need head_share, tail_share >= 0 and head_share + tail_share <= 1")
	case c.AnomalyStd <= 0 || c.RelevanceK < 0:
		return errors.New("need anomaly_std > 0 and relevance_k >= 0")
	}
	return nil
}
