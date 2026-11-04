package inproc

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

func TestMemoryLedgerStatsAndRecent(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GASCOIGNE_LEDGER", "")
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	e, err := New(ctx, Options{IgnoreUserFile: true, Ledger: LedgerMemory})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close(ctx)
	raw := strings.Repeat("INFO request served path=/api/items status=200 ms=12\n", 400)
	for _, session := range []string{"s1", "s1", "s2"} {
		if _, err := e.Optimize(ctx, Request{Text: raw, Tool: "bash", Cmd: "tail app.log", Session: session, Agent: "sdk", Model: "claude-sonnet-4-5"}); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := e.Stats(ctx, StatsQuery{By: "agent"})
	if err != nil || len(stats) != 1 {
		t.Fatalf("Stats = %+v, %v", stats, err)
	}
	if stats[0].Group != "sdk" || stats[0].Runs != 3 || stats[0].Saved <= 0 || stats[0].Saved != stats[0].RawTokens-stats[0].OutTokens {
		t.Fatalf("stat = %+v", stats[0])
	}
	if one, _ := e.Stats(ctx, StatsQuery{By: "agent", Session: "s2"}); len(one) != 1 || one[0].Runs != 1 {
		t.Fatalf("session filter: %+v", one)
	}
	if _, err := e.Stats(ctx, StatsQuery{By: "nonsense"}); err == nil {
		t.Fatal("unknown grouping accepted")
	}

	runs, err := e.Recent(ctx, RecentQuery{Session: "s1", Limit: 10})
	if err != nil || len(runs) != 2 {
		t.Fatalf("Recent = %+v, %v", runs, err)
	}
	if runs[0].ID <= runs[1].ID || runs[0].Session != "s1" || runs[0].RawID == "" || runs[0].Outcome != "optimized" || runs[0].Time.IsZero() {
		t.Fatalf("run = %+v", runs[0])
	}
	if entries, _ := os.ReadDir(data); len(entries) != 0 {
		t.Fatalf("memory ledger wrote %d entries to the data dir", len(entries))
	}
}

func TestLedgerOptionRejectsUnknownMode(t *testing.T) {
	if _, err := New(context.Background(), Options{IgnoreUserFile: true, Ledger: "disk"}); err == nil {
		t.Fatal("unknown ledger mode accepted")
	}
}

func TestFileLedgerIsTheDefault(t *testing.T) {
	ctx := context.Background()
	e, err := New(ctx, opts(t))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close(ctx)
	if got := e.led.Path(); got == ":memory:" {
		t.Fatalf("default ledger is in memory: %s", got)
	}
}

func costEngine(t *testing.T, o Options) *Engine {
	t.Helper()
	t.Setenv("GASCOIGNE_LEDGER", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	o.IgnoreUserFile, o.Ledger = true, LedgerMemory
	ctx := context.Background()
	e, err := New(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close(ctx) })
	raw := strings.Repeat("INFO request served path=/api/items status=200 ms=12\n", 400)
	if _, err := e.Optimize(ctx, Request{Text: raw, Tool: "bash", Cmd: "tail app.log", Agent: "sdk", Model: "acme-large"}); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestCostsUseThePricesTheHostGives(t *testing.T) {
	t.Cleanup(func() { cost.SetOverrides(nil); cost.SetBilling("") })
	in, out := 3.0, 15.0
	e := costEngine(t, Options{Pricing: &Pricing{Prices: map[string]Price{"acme-large": {In: &in, Out: &out, Tokenizer: "claude"}}}})
	costs, err := e.Costs(context.Background(), StatsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if costs.Billing != "api" || len(costs.Savings) != 1 || len(costs.Unpriced) != 0 {
		t.Fatalf("costs = %+v", costs)
	}
	saving := costs.Savings[0]
	if saving.Model != "acme-large" || saving.Source != "static" || saving.Tokens <= 0 || costs.InContextUSD <= 0 || costs.FirstReadUSD <= 0 {
		t.Fatalf("saving = %+v", saving)
	}
}

func TestCostsWithoutPricesListsTheModelUnpriced(t *testing.T) {
	t.Cleanup(func() { cost.SetOverrides(nil); cost.SetBilling("") })
	e := costEngine(t, Options{})
	costs, err := e.Costs(context.Background(), StatsQuery{})
	if err != nil || len(costs.Savings) != 0 || len(costs.Unpriced) != 1 || costs.Unpriced[0] != "acme-large" {
		t.Fatalf("costs = %+v, %v", costs, err)
	}
}

func TestSubscriptionHostGetsNoDollars(t *testing.T) {
	t.Cleanup(func() { cost.SetOverrides(nil); cost.SetBilling("") })
	e := costEngine(t, Options{Pricing: &Pricing{Billing: "subscription"}})
	costs, err := e.Costs(context.Background(), StatsQuery{})
	if err != nil || costs.Billing != "subscription" || len(costs.Savings) != 0 || costs.InContextUSD != 0 {
		t.Fatalf("costs = %+v, %v", costs, err)
	}
}

func TestPricingFileAndBadPrices(t *testing.T) {
	t.Cleanup(func() { cost.SetOverrides(nil); cost.SetBilling("") })
	dir := t.TempDir()
	file := dir + "/sheet.json"
	os.WriteFile(file, []byte(`{"models":{"acme-large":{"in":1,"out":2}}}`), 0o600)
	e := costEngine(t, Options{Pricing: &Pricing{File: file}})
	costs, err := e.Costs(context.Background(), StatsQuery{})
	if err != nil || len(costs.Savings) != 1 || costs.Savings[0].Source != "json" {
		t.Fatalf("costs = %+v, %v", costs, err)
	}
	neg := -1.0
	one := 1.0
	for name, p := range map[string]*Pricing{
		"negative":     {Prices: map[string]Price{"x": {In: &neg, Out: &one}}},
		"missing out":  {Prices: map[string]Price{"x": {In: &one}}},
		"missing file": {File: dir + "/none.json"},
		"bad billing":  {Billing: "free"},
	} {
		if _, err := New(context.Background(), Options{IgnoreUserFile: true, Pricing: p}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}
