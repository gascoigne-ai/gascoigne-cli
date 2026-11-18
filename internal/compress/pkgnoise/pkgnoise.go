// Package pkgnoise compresses package manager output: progress, downloads,
// cache hits and boilerplate go; repeated warnings fold into one line per
// kind; summaries, added/removed packages, vulnerabilities and errors stay.
package pkgnoise

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

// Rules describe a manager.
type Rules struct {
	Noise pcomb.Rules // dropped
	// Fold groups lines into "kind (N): items"; group 1 is the item.
	Fold []Fold
}

// Fold is a rule that groups the lines matching Re under one kind.
type Fold struct {
	Kind string
	Re   *lazyre.Regexp
}

// For returns the rules of a manager; ok is false when unknown.
func For(dialect string) (Rules, bool) {
	r, ok := rules[dialect]
	return r, ok
}

// Compress drops noise, folds lines by kind (at the first one's position)
// and squeezes blank runs.
func Compress(lines []string, rules Rules) []string {
	var out []string
	at := map[string]int{}
	items := map[string][]string{}
	for _, l := range lines {
		t := strings.TrimRight(l, " \t")
		if rules.Noise.Match(t) {
			continue
		}
		folded := false
		for _, f := range rules.Fold {
			if m := f.Re.FindStringSubmatch(t); m != nil {
				if _, ok := at[f.Kind]; !ok {
					at[f.Kind] = len(out)
					out = append(out, "")
				}
				items[f.Kind] = append(items[f.Kind], m[1])
				folded = true
				break
			}
		}
		if !folded {
			out = append(out, t)
		}
	}
	for kind, i := range at {
		out[i] = fmt.Sprintf("%s (%d): %s", kind, len(items[kind]), strings.Join(items[kind], ", "))
	}
	return compress.SqueezeBlank(compress.DedupeRuns(trim(out)))
}

func trim(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}
