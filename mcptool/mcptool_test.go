package mcptool

import (
	"encoding/json"
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
