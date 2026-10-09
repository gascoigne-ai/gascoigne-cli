package catoptimizer

import (
	"errors"
	"slices"
)

// Modes. Auto picks by size: small files untouched, medium cleaned, large outlined.
const (
	Auto    = "auto"
	Full    = "full"
	Clean   = "clean"
	Outline = "outline"
)

// Per-run overrides of Config.Mode.
const (
	AttrMode = "cat_mode"
	EnvMode  = "GASCOIGNE_CAT_MODE"
)

type Config struct {
	Mode string `yaml:"mode"`
	// CleanMinTokens: auto mode leaves smaller files untouched.
	CleanMinTokens int `yaml:"clean_min_tokens"`
	// OutlineMinTokens: auto mode outlines files at least this big.
	OutlineMinTokens int `yaml:"outline_min_tokens"`
	// MinBody: shorter function bodies are kept even in outlines.
	MinBody int `yaml:"min_body"`
}

func (c *Config) Validate() error {
	if !validMode(c.Mode) {
		return errors.New("mode must be auto, full, clean or outline")
	}
	if c.CleanMinTokens < 0 || c.OutlineMinTokens < c.CleanMinTokens || c.MinBody < 1 {
		return errors.New("need 0 <= clean_min_tokens <= outline_min_tokens and min_body >= 1")
	}
	return nil
}

func validMode(m string) bool {
	return slices.Contains([]string{Auto, Full, Clean, Outline}, m)
}
