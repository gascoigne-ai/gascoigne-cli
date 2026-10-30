package main

import (
	"strings"
	"testing"
)

// The money pages must say what their dollars are: API-equivalent estimates,
// not spend, with the assumed share and the Claude Code scope named.
func TestMoneyPagesSayWhatTheyShow(t *testing.T) {
	for _, c := range []struct {
		page string
		html []byte
		want []string
	}{
		{"costs", costsHTML, []string{"API-equivalent", "recorded no model", "factor not measured"}},
		{"context", contextHTML, []string{"Claude Code only", "main conversation", "Likely cause", "Total with subagents"}},
	} {
		for _, w := range c.want {
			if !strings.Contains(string(c.html), w) {
				t.Errorf("%s page lacks %q", c.page, w)
			}
		}
	}
}
