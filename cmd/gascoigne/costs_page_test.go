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
		{"costs", costsHTML, []string{"API-equivalent", "recorded no model", "factor not measured", "Not subtracted"}},
		{"context", contextHTML, []string{"Claude Code only", "main conversation", "Likely cause", "Total with subagents"}},
	} {
		for _, w := range c.want {
			if !strings.Contains(string(c.html), w) {
				t.Errorf("%s page lacks %q", c.page, w)
			}
		}
	}
}

// Costs keeps to the estimate from saved tokens: no family table that repeats
// the model table, no proxy tables (the Context page shows them), and the same
// periods as Context so the two pages compare.
func TestCostsPageLayout(t *testing.T) {
	page := string(costsHTML)
	for _, gone := range []string{"By model family", "Measured by the proxy", "Rereads assumed", "Last 4 weeks"} {
		if strings.Contains(page, gone) {
			t.Errorf("costs page still has %q", gone)
		}
	}
	for _, p := range []string{`value="24h"`, `value="7d"`, `value="14d" selected`, `value="30d"`, `value="90d"`} {
		if !strings.Contains(page, p) || !strings.Contains(string(contextHTML), strings.ReplaceAll(p, " selected", "")) {
			t.Errorf("period %s missing on a page", p)
		}
	}
	if !strings.Contains(string(contextHTML), "Requests through the proxy") {
		t.Error("context page lacks the proxy requests table")
	}
}
