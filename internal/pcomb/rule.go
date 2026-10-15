package pcomb

import (
	"errors"
	"fmt"
)

// Rule is a matcher with a name and the lines it must and must not match.
// The examples are the rule's documentation and its test: Verify runs them.
type Rule struct {
	Name  string
	Match Matcher
	Yes   []string // lines the rule must match
	No    []string // near misses it must not match
}

// Rules is an ordered rule table; a line matches when any rule matches it.
type Rules []Rule

// Match reports whether any rule matches the line.
func (rs Rules) Match(line string) bool {
	_, ok := rs.First(line)
	return ok
}

// First returns the first rule that matches the line, to tell which one fired.
func (rs Rules) First(line string) (Rule, bool) {
	for _, r := range rs {
		if r.Match(line) {
			return r, true
		}
	}
	return Rule{}, false
}

// Verify checks every rule against its own examples and that names are unique.
func (rs Rules) Verify() error {
	var errs []error
	seen := map[string]bool{}
	for _, r := range rs {
		if r.Name == "" || r.Match == nil {
			errs = append(errs, errors.New("rule needs a Name and a Match"))
			continue
		}
		if seen[r.Name] {
			errs = append(errs, fmt.Errorf("%s: duplicate rule name", r.Name))
		}
		seen[r.Name] = true
		if len(r.Yes) == 0 {
			errs = append(errs, fmt.Errorf("%s: needs at least one Yes example", r.Name))
		}
		for _, l := range r.Yes {
			if !r.Match(l) {
				errs = append(errs, fmt.Errorf("%s: should match %q", r.Name, l))
			}
		}
		for _, l := range r.No {
			if r.Match(l) {
				errs = append(errs, fmt.Errorf("%s: should not match %q", r.Name, l))
			}
		}
	}
	return errors.Join(errs...)
}
