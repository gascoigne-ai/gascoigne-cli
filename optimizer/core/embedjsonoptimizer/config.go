package embedjsonoptimizer

import (
	"errors"

	"github.com/gascoigne-ai/gascoigne-cli/compress/jsonx"
)

// Config picks the text blocks searched for JSON and how JSON is crushed.
type Config struct {
	// Types lists types or families whose text may wrap JSON. Data, cloud and
	// http blocks are JSON engines' own; diffs and source keep JSON verbatim.
	Types []string `yaml:"types"`
	// JSON crushes each embedded value, like the json optimizer.
	JSON jsonx.Config `yaml:"json"`
}

func (c *Config) Validate() error {
	if len(c.Types) == 0 {
		return errors.New("types must not be empty")
	}
	return c.JSON.Validate()
}
