// Package router picks the optimizer chain for a payload.
package router

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/internal/payload"
)

const DefaultRoute = "default"

// Match conditions are ANDed; zero values match anything.
type Match struct {
	Attrs     map[string]string `yaml:"attrs"`
	Kind      payload.Kind      `yaml:"kind"`
	MinTokens int               `yaml:"min_tokens"`
}

type Route struct {
	Name       string   `yaml:"name"`
	Match      Match    `yaml:"match"`
	Optimizers []string `yaml:"optimizers"`
}

type Config struct {
	Routes  []Route  `yaml:"routes"`
	Default []string `yaml:"default"`
}

// Optimizers lists every optimizer ID the config references.
func (c *Config) Optimizers() []string {
	ids := slices.Clone(c.Default)
	for _, r := range c.Routes {
		ids = append(ids, r.Optimizers...)
	}
	return ids
}

// Route returns the first matching route's name and optimizer chain, else the default.
func (c *Config) Route(p *payload.Payload) (string, []string) {
	for i, r := range c.Routes {
		if r.Match.matches(p) {
			return r.label(i), r.Optimizers
		}
	}
	return DefaultRoute, c.Default
}

func (r Route) label(i int) string {
	if r.Name != "" {
		return r.Name
	}
	return fmt.Sprintf("route%d", i)
}

func (m Match) matches(p *payload.Payload) bool {
	if m.Kind != "" && m.Kind != p.Kind() {
		return false
	}
	if m.MinTokens > 0 && p.Tokens() < m.MinTokens {
		return false
	}
	for k, pattern := range m.Attrs {
		v, ok := p.Attrs[k]
		if !ok || !Glob(pattern, v) {
			return false
		}
	}
	return true
}

// Glob matches s against pattern where '*' matches any run of characters,
// including '/' (unlike path.Match, since commands and paths contain slashes).
func Glob(pattern, s string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == s
	}
	first, last := parts[0], parts[len(parts)-1]
	if !strings.HasPrefix(s, first) {
		return false
	}
	s = s[len(first):]
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(s, part)
		if i < 0 {
			return false
		}
		s = s[i+len(part):]
	}
	return strings.HasSuffix(s, last)
}
