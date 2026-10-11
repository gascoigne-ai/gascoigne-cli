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
			{"claude", "claude-opus-5-5", 2, 15, 18300, 340, 200},
			{"codex", "gpt-5.3-codex", 1, 2000, 1000, 0, 50},
		}},
		{"since", component.StatsQuery{Since: now.Add(-time.Hour)}, []UsageStat{{"claude", "claude-opus-5-5", 2, 15, 18300, 340, 200}}},
		{"agent", component.StatsQuery{Agent: "codex"}, []UsageStat{{"codex", "gpt-5.3-codex", 1, 2000, 1000, 0, 50}}},
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
