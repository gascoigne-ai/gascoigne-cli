package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
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

// gain -pipeline NAME[,NAME] sums only those pipelines and -by pipeline|type
// groups by them, against a throwaway ledger.
func TestGainPipelineFlag(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	path := filepath.Join(t.TempDir(), "l.db")
	t.Setenv(ledger.EnvPath, path)
	store := ledger.New(&ledger.Config{Path: path})
	if err := store.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	for i, ev := range []component.Event{
		{Pipeline: "exec", Command: "go test", Type: "test/go", RawTokens: 1000, OutTokens: 100},
		{Pipeline: "hook", Command: "git log", RawTokens: 500, OutTokens: 500},
		{Pipeline: "eval", Command: "go test", Type: "test/go", RawTokens: 4000, OutTokens: 40},
	} {
		ev.TS, ev.Key, ev.Outcome = now.Add(-time.Duration(i)*time.Minute), "k"+ev.Pipeline, component.Unchanged
		if err := store.Record(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	gain := func(args ...string) string {
		t.Helper()
		var err error
		out := capture(t, func() { err = cmdGain(t.Context(), append(args, "-csv", "-all")) })
		if err != nil {
			t.Fatalf("gain %v: %v", args, err)
		}
		return out
	}
	// total row: runs,optimized,failed,in,out (columns 2 to 6 of the csv)
	if out := gain("-by", "command", "-since", "7d", "-pipeline", "exec,hook"); !strings.Contains(out, "total,2,0,0,1500,600,") {
		t.Errorf("exec,hook total:\n%s", out)
	}
	if out := gain("-by", "pipeline"); !strings.Contains(out, "eval,1,") || !strings.Contains(out, "total,3,") {
		t.Errorf("by pipeline:\n%s", out)
	}
	if out := gain("-by", "type"); !strings.Contains(out, "test/go,2,") || !strings.Contains(out, "\n-,1,") {
		t.Errorf("by type:\n%s", out)
	}
}
