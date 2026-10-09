package mcptool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lukaszb/gascoigne/component"
)

func TestReport(t *testing.T) {
	ts := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	r := Report(component.StatsQuery{By: component.ByFamily, Session: "s1"}, []component.Stat{
		{Group: "js", Runs: 2, Optimized: 2, RawTokens: 300, OutTokens: 100, First: ts, Last: ts},
		{Group: "data", Runs: 1, Optimized: 1, RawTokens: 100, OutTokens: 99, First: ts, Last: ts},
	})
	got, _ := json.Marshal(r)
	want := `{"by":"family","session":"s1","groups":[` +
		`{"group":"js","runs":2,"optimized":2,"failed":0,"raw_tokens":300,"out_tokens":100,"saved_tokens":200,"saved_pct":66.7,"first":"2026-10-01T12:00:00Z","last":"2026-10-01T12:00:00Z"},` +
		`{"group":"data","runs":1,"optimized":1,"failed":0,"raw_tokens":100,"out_tokens":99,"saved_tokens":1,"saved_pct":1,"first":"2026-10-01T12:00:00Z","last":"2026-10-01T12:00:00Z"}],` +
		`"total":{"group":"total","runs":3,"optimized":3,"failed":0,"raw_tokens":400,"out_tokens":199,"saved_tokens":201,"saved_pct":50.3}}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestReportEmpty(t *testing.T) {
	got, _ := json.Marshal(Report(component.StatsQuery{}, nil))
	if want := `{"by":"route","groups":[],"total":{"group":"total","runs":0,"optimized":0,"failed":0,"raw_tokens":0,"out_tokens":0,"saved_tokens":0,"saved_pct":0}}`; string(got) != want {
		t.Fatalf("got %s", got)
	}
}

type fakeLedger struct{ component.Ledger }

func (fakeLedger) Stats(_ context.Context, q component.StatsQuery) ([]component.Stat, error) {
	switch q.By {
	case component.ByOutcome:
		return []component.Stat{{Group: "optimized", Runs: 1, RawTokens: 100, OutTokens: 10}, {Group: "bypassed", Runs: 2, RawTokens: 30, OutTokens: 30}}, nil
	case component.ByCommand:
		return []component.Stat{{Group: "go test", Runs: 1, Optimized: 1, RawTokens: 100, OutTokens: 10}, {Group: "ls", Runs: 2, RawTokens: 30, OutTokens: 30}}, nil
	case component.ByOptimizer:
		return []component.Stat{{Group: "go", Family: "go", Runs: 1, RawTokens: 100, OutTokens: 10}}, nil
	case component.BySkip:
		return []component.Stat{{Group: "minsize", Runs: 2, RawTokens: 30, OutTokens: 30}}, nil
	}
	return nil, nil
}

func TestOverview(t *testing.T) {
	o, err := BuildOverview(t.Context(), fakeLedger{}, "s1")
	if err != nil {
		t.Fatal(err)
	}
	table := o.Table()
	for _, want := range []string{
		"session s1",
		"3 runs · 1 optimized · 2 skipped · 0 failed · 130 → 40 tokens · saved 90 (69.2%)",
		"by outcome", "bypassed", "by command", "go test", "by optimizer", "go         go",
		"skipped", "minsize", "below min_tokens",
	} {
		if !strings.Contains(table, want) {
			t.Errorf("missing %q in:\n%s", want, table)
		}
	}
	data, _ := json.Marshal(o)
	for _, key := range []string{`"outcomes":`, `"commands":`, `"optimizers":`, `"skipped":`, `"family":"go"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("json missing %s: %s", key, data)
		}
	}
}
