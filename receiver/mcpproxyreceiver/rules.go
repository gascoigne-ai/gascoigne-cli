package mcpproxyreceiver

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

// Action is what a rule does to the tools it matches. The constants stay untyped so callers that hold
// them as plain strings (cmd/gascoigne suggestions) keep compiling.
type Action string

const (
	ActionShrink = "shrink" // optimize the results (the default; ends the search, so it overrides a later skip)
	ActionSkip   = "skip"   // pass the results through untouched
	ActionLazy   = "lazy"   // list the tool on request only
)

var actions = []Action{ActionShrink, ActionSkip, ActionLazy}

// Rule is one manual setting. Match is a glob (path.Match) on the downstream
// tool name, or, with a slash, on "server/tool" where server is the name the
// server reports. Rules are tried in order and the first match wins.
type Rule struct {
	Match  string `yaml:"match" json:"match"`
	Action Action `yaml:"action" json:"action"`
}

func joinActions() string {
	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = string(a)
	}
	return strings.Join(names, ", ")
}

// Validate reports a rule that cannot work: an unknown action or a bad glob.
func (r Rule) Validate() error {
	if !slices.Contains(actions, r.Action) {
		return fmt.Errorf("rule %q: action must be one of %s, not %q", r.Match, joinActions(), r.Action)
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
func Resolve(rules []Rule, server, tool string) Action {
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

// WithRules sets the rules saved outside the config (the review page's), by
// server id. They are tried after the config's own.
func WithRules(rules map[string][]Rule) Option {
	return func(rec *Receiver) { rec.rules = rules }
}

// rulesFor are the rules of this server: the config's first, then the saved ones.
func (r *Receiver) rulesFor() []Rule {
	return slices.Concat(r.cfg.Rules, r.rules[r.server])
}
