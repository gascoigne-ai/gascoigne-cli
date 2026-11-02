package logoptimizer

import (
	"errors"
	"fmt"
	"slices"
)

type Config struct {
	// Context lines kept around warnings and errors.
	Context int `yaml:"context"`
	// DropDebug removes DEBUG and TRACE lines.
	DropDebug bool `yaml:"drop_debug"`
	// MaxLines caps the result; warnings and errors are kept first.
	MaxLines int `yaml:"max_lines"`
	// Passes lists the stages that run on a log before its lines are
	// templated, in this order: entries (structured events become lines),
	// units (systemd Starting/Started pairs), prefixes (container prefixes
	// are cut and relabeled). Drop one to turn it off; order is fixed.
	Passes []string `yaml:"passes"`
	// Bursts names the lines where most repeats of a template sit when they
	// are packed into a short stretch of the log.
	Bursts bool `yaml:"bursts"`
	// Outliers picks how a number far from its template's usual value is
	// found: ratio (at least 10x the median) or mad (also a modified z-score
	// above 3.5 and 1.5x the median, so a 4x latency spike stays).
	Outliers string `yaml:"outliers"`
	// Rarity spends the room left under MaxLines on the templates that stand
	// for the fewest lines, not only on head, tail and errors.
	Rarity bool `yaml:"rarity"`
	// Miner picks how lines are told to repeat: regex (numbers, ids and
	// timestamps masked, exact match) or drain (also lines that differ in a few words).
	Miner string `yaml:"miner"`
	// DrainDepth is the miner tree depth: the length level, then depth-2 leading tokens.
	DrainDepth int `yaml:"drain_depth"`
	// DrainSimilarity is the share of equal words a line needs to join a template.
	DrainSimilarity float64 `yaml:"drain_similarity"`
	// DrainMaxChildren caps the branches of a tree node.
	DrainMaxChildren int `yaml:"drain_max_children"`
	// DrainMaxVars is the most words one template may vary in (0: no limit);
	// lines of free text with more differing words stay apart.
	DrainMaxVars int `yaml:"drain_max_vars"`
	// DrainVariants is how many word variants of one template (a user, a
	// device) keep a line of their own; the rest fold into one line that names them.
	DrainVariants int `yaml:"drain_variants"`
}

// defaultPasses is every stage, in the only order they can run: a later one
// reads what an earlier one wrote.
var defaultPasses = []string{"entries", "units", "prefixes"}

const (
	outliersRatio = "ratio"
	outliersMAD   = "mad"
	minerRegex    = "regex"
	minerDrain    = "drain"
)

func (c *Config) Validate() error {
	if c.Context < 0 || c.MaxLines < 20 {
		return errors.New("context must be >= 0 and max_lines >= 20")
	}
	last := -1
	for _, name := range c.Passes {
		i := slices.Index(defaultPasses, name)
		if i < 0 {
			return fmt.Errorf("passes: unknown %q, want some of %v in that order", name, defaultPasses)
		}
		if i <= last {
			return fmt.Errorf("passes: %q is repeated or out of order, want %v", name, defaultPasses)
		}
		last = i
	}
	switch c.Outliers {
	case "", outliersRatio, outliersMAD:
	default:
		return fmt.Errorf("outliers %q: want %s or %s", c.Outliers, outliersRatio, outliersMAD)
	}
	switch c.Miner {
	case "", minerRegex:
	case minerDrain:
		if c.DrainDepth < 3 || c.DrainSimilarity <= 0 || c.DrainSimilarity > 1 || c.DrainMaxChildren < 1 || c.DrainVariants < 1 || c.DrainMaxVars < 0 {
			return errors.New("drain needs drain_depth >= 3, drain_similarity in (0,1] and drain_max_children >= 1 and drain_variants >= 1")
		}
	default:
		return fmt.Errorf("miner %q: want %s or %s", c.Miner, minerRegex, minerDrain)
	}
	return nil
}
