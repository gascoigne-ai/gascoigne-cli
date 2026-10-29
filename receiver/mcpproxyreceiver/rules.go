package mcpproxyreceiver

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
)

// What a rule does to the tools it matches.
const (
	ActionShrink = "shrink" // optimize the results (the default; ends the search, so it overrides a later skip)
	ActionSkip   = "skip"   // pass the results through untouched
	ActionLazy   = "lazy"   // list the tool on request only
)

var actions = []string{ActionShrink, ActionSkip, ActionLazy}

// Rule is one manual setting. Match is a glob (path.Match) on the downstream
// tool name, or, with a slash, on "server/tool" where server is the name the
// server reports. Rules are tried in order and the first match wins.
type Rule struct {
	Match  string `yaml:"match" json:"match"`
	Action string `yaml:"action" json:"action"`
}

// Validate reports a rule that cannot work: an unknown action or a bad glob.
func (r Rule) Validate() error {
	if !slices.Contains(actions, r.Action) {
		return fmt.Errorf("rule %q: action must be one of %s, not %q", r.Match, strings.Join(actions, ", "), r.Action)
	}
	if r.Match == "" {
		return fmt.Errorf("rule with action %s: match must be set", r.Action)
	}
	if _, err := path.Match(r.Match, ""); err != nil {
		return fmt.Errorf("rule %q: %w", r.Match, err)
	}
	return nil
}

// Resolve is the action for a tool of a server (id: name or name#hash): that
// of the first matching rule, "" when none matches.
func Resolve(rules []Rule, server, tool string) string {
	name, _, _ := strings.Cut(server, "#")
	for _, r := range rules {
		target := tool
		if strings.Contains(r.Match, "/") {
			target = name + "/" + tool
		}
		if ok, _ := path.Match(r.Match, target); ok {
			return r.Action
		}
	}
	return ""
}

type rulesKey struct{}

// WithRules sets the rules saved outside the config (the review page's), by
// server id. They are tried after the config's own.
func WithRules(ctx context.Context, rules map[string][]Rule) context.Context {
	return context.WithValue(ctx, rulesKey{}, rules)
}

// rulesFor are the rules of this server: the config's first, then the saved ones.
func (r *Receiver) rulesFor(ctx context.Context) []Rule {
	saved, _ := ctx.Value(rulesKey{}).(map[string][]Rule)
	return slices.Concat(r.cfg.Rules, saved[r.server])
}
