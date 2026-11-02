package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
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

// gain -refetch shows, per family, how often the agent fetched the original
// after an optimized result: the rate the history filter turns gentle on.
func TestGainRefetch(t *testing.T) {
	l, _ := testDashboard(t)
	var b strings.Builder
	if err := printRefetch(t.Context(), &b, l, component.StatsQuery{}); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); !strings.Contains(got, "no optimized runs") {
		t.Errorf("empty ledger: %q", got)
	}
	ctx := t.Context()
	now := time.Now()
	step := func(fam string) []component.Step {
		return []component.Step{{Optimizer: fam + "x", Family: fam, RawTokens: 100, OutTokens: 10}}
	}
	for i, ev := range []component.Event{
		{Key: "a", Outcome: component.Optimized, Steps: step("log")},
		{Key: "b", Outcome: component.Optimized, Steps: step("log")},
		{Key: "c", Outcome: component.Optimized, Steps: step("log")},
		{Key: "d", Outcome: component.Optimized, Steps: step("log")},
		{Key: "e", Outcome: component.Optimized, Steps: step("test")},
	} {
		ev.TS, ev.RawTokens, ev.OutTokens = now.Add(-time.Hour+time.Duration(i)*time.Second), 100, 10
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []string{"a", "e"} {
		if err := l.MarkFailure(ctx, k, component.ReasonRawRequested); err != nil {
			t.Fatal(err)
		}
	}
	b.Reset()
	if err := printRefetch(ctx, &b, l, component.StatsQuery{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	// highest rate first: test 1 of 1, then log 1 of 4
	if len(lines) != 3 || !strings.HasPrefix(lines[1], "test") || !strings.Contains(lines[1], "100%") ||
		!strings.HasPrefix(lines[2], "log") || !strings.Contains(lines[2], "25%") {
		t.Errorf("table:\n%s", b.String())
	}
}
