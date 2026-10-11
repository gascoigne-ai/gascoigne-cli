package detectfilter

import "errors"

type Config struct {
	// EditSafe lists families whose files, read from disk (cat, sed -n,
	// head, an agent's Read through the LLM proxy) and smaller than
	// EditSafeMaxTokens, are typed as source (fs/code): an agent reads a
	// config to edit it, which needs its own syntax and exact lines, not
	// JSON tables. Bigger ones (lockfiles, dumps) keep their data engines.
	EditSafe          []string `yaml:"edit_safe"`
	EditSafeMaxTokens int      `yaml:"edit_safe_max_tokens"`
}

func (c *Config) Validate() error {
	if c.EditSafeMaxTokens < 0 {
		return errors.New("edit_safe_max_tokens must not be negative")
	}
	return nil
}
