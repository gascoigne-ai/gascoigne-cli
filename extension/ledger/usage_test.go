package ledger

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

func TestUsageStats(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	ctx := t.Context()
	var _ component.UsageRecorder = l
	now := time.Now()
	for _, u := range []component.Usage{
		{TS: now, Agent: "claude", Model: "claude-opus-5-5", Input: 10, CacheRead: 9000, CacheWrite: 300, Output: 120},
		{TS: now, Agent: "claude", Model: "claude-opus-5-5", Input: 5, CacheRead: 9300, CacheWrite: 40, Output: 80},
		{TS: now.Add(-48 * time.Hour), Agent: "codex", Model: "gpt-5.3-codex", Input: 2000, CacheRead: 1000, Output: 50},
	} {
		if err := l.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name string
		q    component.StatsQuery
		want []UsageStat
	}{
		{"all", component.StatsQuery{}, []UsageStat{
			{"claude", "claude-opus-5-5", 2, 15, 18300, 340, 0, 200, 9327, 9345},
			{"codex", "gpt-5.3-codex", 1, 2000, 1000, 0, 0, 50, 3000, 3000},
		}},
		{"since", component.StatsQuery{Since: now.Add(-time.Hour)}, []UsageStat{{"claude", "claude-opus-5-5", 2, 15, 18300, 340, 0, 200, 9327, 9345}}},
		{"agent", component.StatsQuery{Agent: "codex"}, []UsageStat{{"codex", "gpt-5.3-codex", 1, 2000, 1000, 0, 0, 50, 3000, 3000}}},
		{"session has none", component.StatsQuery{Session: "s1"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := l.UsageStats(ctx, tt.q)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %+v, want %+v", got[i], tt.want[i])
				}
			}
		})
	}

	if _, err := l.Reset(ctx, component.StatsQuery{Agent: "codex"}, false); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.UsageStats(ctx, component.StatsQuery{}); len(got) != 1 || got[0].Agent != "claude" {
		t.Fatalf("after reset: %+v", got)
	}
}

func TestUsageDays(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	ctx := t.Context()
	day := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	for _, u := range []component.Usage{
		{TS: day, Agent: "claude", Model: "m", Input: 10, CacheRead: 9000, CacheWrite: 300, Output: 100},
		{TS: day.Add(time.Hour), Agent: "claude", Model: "m", Input: 5, CacheRead: 9300, CacheWrite: 40, Output: 80},
		{TS: day.Add(time.Hour), Agent: "codex", Model: "g", Input: 2000, CacheRead: 1000, Output: 50},
		{TS: day.AddDate(0, 0, 2), Agent: "claude", Model: "m", Input: 1, CacheRead: 20000, CacheWrite: 0, Output: 10},
	} {
		if err := l.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	got, err := l.UsageDays(ctx, component.StatsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	want := []UsageDay{
		{"2026-10-10", 3, 19300, 340, 0, 230, (9310 + 9345 + 3000) / 3, 9345},
		{"2026-10-12", 1, 20000, 0, 0, 10, 20001, 20001},
	}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got, _ := l.UsageDays(ctx, component.StatsQuery{Agent: "codex"}); len(got) != 1 || got[0].Requests != 1 {
		t.Fatalf("agent filter: %+v", got)
	}
	if got, _ := l.UsageDays(ctx, component.StatsQuery{Session: "s1"}); len(got) != 0 {
		t.Fatalf("a session filter matches nothing: %+v", got)
	}
}

func TestUsageKeepsTheHourWrites(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	ctx := t.Context()
	now := time.Now()
	for _, u := range []component.Usage{
		{TS: now, Agent: "claude", Model: "m", CacheWrite: 1000, CacheWrite1h: 600},
		{TS: now, Agent: "claude", Model: "m", CacheWrite: 500},
	} {
		if err := l.RecordUsage(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := l.UsageStats(ctx, component.StatsQuery{})
	if err != nil || len(stats) != 1 || stats[0].CacheWrite != 1500 || stats[0].CacheWrite1h != 600 {
		t.Fatalf("%+v %v", stats, err)
	}
	days, err := l.UsageDays(ctx, component.StatsQuery{})
	if err != nil || len(days) != 1 || days[0].CacheWrite1h != 600 {
		t.Fatalf("%+v %v", days, err)
	}
}
