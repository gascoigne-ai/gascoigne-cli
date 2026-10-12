package main

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/cost"
)

func TestGainOneline(t *testing.T) {
	l, _ := testDashboard(t)
	var b strings.Builder
	if err := printOneline(t.Context(), &b, l, component.StatsQuery{}, cost.AgentContext); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != "gascoigne: 0 tokens saved (0%)\n" {
		t.Errorf("empty ledger: %q", got)
	}
	record(t, l, "s1", "git log")
	b.Reset()
	if err := printOneline(t.Context(), &b, l, component.StatsQuery{Session: "s1"}, cost.AgentContext); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	if !strings.HasPrefix(got, "gascoigne: 900 tokens saved (90%)") || strings.Count(got, "\n") != 1 {
		t.Errorf("one line with the totals: %q", got)
	}
}
