package ledger

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
)

func TestToolStats(t *testing.T) {
	cfg := &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour, StatsTTL: 14 * 24 * time.Hour}
	l := start(t, cfg)
	var _ mcptool.ToolStatRecorder = l
	ctx := t.Context()
	day := func(n int) time.Time { return time.Date(2026, 10, 1+n, 12, 0, 0, 0, time.UTC) }

	record := func(d time.Time, calls map[string]int, sizes map[string]int) {
		t.Helper()
		l.now = func() time.Time { return d }
		if err := l.RecordToolList(ctx, "claude", "github", sizes, nil); err != nil {
			t.Fatal(err)
		}
		for tool, n := range calls {
			for range n {
				if err := l.RecordToolCall(ctx, "claude", "github", tool); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	record(day(0), map[string]int{"search": 3}, map[string]int{"search": 120, "create_issue": 340})
	record(day(0), map[string]int{"search": 2}, map[string]int{"search": 125}) // a later listing the same day
	record(day(1), nil, map[string]int{"search": 125, "create_issue": 340})

	t.Run("upserts per day", func(t *testing.T) {
		got, err := l.ToolStats(ctx, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		want := []ToolStat{
			{"2026-10-01", "claude", "github", "create_issue", 340, 0, false},
			{"2026-10-01", "claude", "github", "search", 125, 5, false}, // size of the last listing, both calls kept
			{"2026-10-02", "claude", "github", "create_issue", 340, 0, false},
			{"2026-10-02", "claude", "github", "search", 125, 0, false},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %+v\nwant %+v", got, want)
		}
	})
	t.Run("a call of an unlisted tool keeps its calls", func(t *testing.T) {
		l.now = func() time.Time { return day(1) }
		if err := l.RecordToolCall(ctx, "claude", "remote", "ping"); err != nil {
			t.Fatal(err)
		}
		got, _ := l.ToolStats(ctx, day(1))
		last := got[len(got)-1]
		if last.Server != "remote" || last.Calls != 1 || last.Tokens != 0 {
			t.Fatalf("got %+v", last)
		}
	})
	t.Run("since", func(t *testing.T) {
		got, _ := l.ToolStats(ctx, day(1))
		for _, s := range got {
			if s.Day < "2026-10-02" {
				t.Fatalf("row before since: %+v", s)
			}
		}
	})
	t.Run("prune drops days before the cutoff's day", func(t *testing.T) {
		l.now = func() time.Time { return day(0).Add(15 * 24 * time.Hour) } // 10-16: cutoff is 10-02
		if err := l.pruneToolStats(ctx); err != nil {
			t.Fatal(err)
		}
		got, _ := l.ToolStats(ctx, time.Time{})
		if len(got) == 0 || got[0].Day != "2026-10-02" {
			t.Fatalf("after prune: %+v", got)
		}
	})
	t.Run("zero ttl keeps all", func(t *testing.T) {
		l.cfg.StatsTTL = 0
		l.now = func() time.Time { return day(400) }
		if err := l.pruneToolStats(ctx); err != nil {
			t.Fatal(err)
		}
		if got, _ := l.ToolStats(ctx, time.Time{}); len(got) == 0 {
			t.Fatal("rows were pruned with stats_ttl 0")
		}
	})
}

func TestPruneEvents(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), EventsTTL: 24 * time.Hour})
	ctx := t.Context()
	now := time.Now()
	l.now = func() time.Time { return now }
	for _, ago := range []time.Duration{48 * time.Hour, time.Hour} {
		ev := component.Event{TS: now.Add(-ago), Key: ago.String(), Outcome: component.Optimized,
			Steps: []component.Step{{Optimizer: "x", RawTokens: 10, OutTokens: 5}}}
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
		if err := l.RecordUsage(ctx, component.Usage{TS: now.Add(-ago), Agent: "a", Model: "m", Input: 1}); err != nil {
			t.Fatal(err)
		}
	}
	l.db.Create(&failure{Key: "old", Count: 1, LastTS: now.Add(-48 * time.Hour).UnixMilli(), Reason: "x"})
	l.db.Create(&failure{Key: "new", Count: 1, LastTS: now.UnixMilli(), Reason: "x"})
	if err := l.pruneEvents(ctx); err != nil {
		t.Fatal(err)
	}
	var events, steps, usages, failures int64
	l.db.Model(&event{}).Count(&events)
	l.db.Model(&step{}).Count(&steps)
	l.db.Model(&usage{}).Count(&usages)
	l.db.Model(&failure{}).Count(&failures)
	if events != 1 || steps != 1 || usages != 1 || failures != 1 {
		t.Errorf("kept events %d steps %d usage %d failures %d, want 1 each", events, steps, usages, failures)
	}
	l.cfg.EventsTTL = 0
	l.now = func() time.Time { return now.Add(1000 * time.Hour) }
	if err := l.pruneEvents(ctx); err != nil {
		t.Fatal(err)
	}
	if l.db.Model(&event{}).Count(&events); events != 1 {
		t.Error("events pruned with events_ttl 0")
	}
}

func TestStatsTTLValidate(t *testing.T) {
	if err := (&Config{StatsTTL: -time.Hour}).Validate(); err == nil {
		t.Fatal("negative stats_ttl accepted")
	}
	if d := NewFactory().DefaultConfig().(*Config).StatsTTL; d != DefaultStatsTTL {
		t.Fatalf("default %v", d)
	}
}

func TestMCPCommands(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	ctx := t.Context()
	at := time.Date(2026, 10, 12, 15, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		cmd string
		ts  time.Time
	}{
		{"mcp__db__query", at},
		{"mcp__db__query", at.Add(time.Hour)},
		{"mcp__db__query", at.AddDate(0, 0, -3)},
		{"mcp__gascoigne__fetch api.github.com", at}, // own tool with a host: the command is the first word
		{"go test", at},                              // not an MCP tool
		{"mcp_db_query", at},                         // Gemini naming: server and tool cannot be told apart
	} {
		err := l.Record(ctx, component.Event{TS: c.ts, Agent: "claude", Pipeline: "hook", Key: "k", Route: "default",
			Command: c.cmd, RawTokens: 10, OutTokens: 10, Outcome: component.Unchanged})
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := l.MCPCommands(ctx, at.AddDate(0, 0, -5))
	if err != nil {
		t.Fatal(err)
	}
	want := []CommandDay{
		{"2026-10-09", "claude", "mcp__db__query", 1},
		{"2026-10-12", "claude", "mcp__db__query", 2},
		{"2026-10-12", "claude", "mcp__gascoigne__fetch", 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got, _ := l.MCPCommands(ctx, at); len(got) != 2 {
		t.Fatalf("since: %+v", got)
	}
}

func TestRecentRuns(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	ctx := t.Context()
	rec := func(session, key, raw string, outcome component.Outcome) {
		t.Helper()
		err := l.Record(ctx, component.Event{TS: time.Now(), Session: session, Key: key, Route: "default", Command: "c",
			RawTokens: 1000, OutTokens: 100, RawID: raw, Outcome: outcome})
		if err != nil {
			t.Fatal(err)
		}
	}
	rec("s1", "cmd:a", "id1", component.Optimized)
	rec("s1", "cmd:b", "id2", component.Optimized)
	rec("s1", "cmd:a", "id3", component.Optimized) // the same key later: it wins
	rec("s1", "cmd:c", "", component.Optimized)    // no original kept
	rec("s1", "cmd:d", "id4", component.Unchanged) // not shortened
	rec("s2", "cmd:e", "id5", component.Optimized) // another session
	got, err := l.RecentRuns(ctx, "s1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != "cmd:a" || got[0].RawID != "id3" || got[1].RawID != "id2" {
		t.Fatalf("got %+v", got)
	}
	if got, _ := l.RecentRuns(ctx, "s1", 1); len(got) != 1 || got[0].RawID != "id3" {
		t.Fatalf("limit: %+v", got)
	}
	if got, _ := l.RecentRuns(ctx, "none", 5); len(got) != 0 {
		t.Fatalf("unknown session: %+v", got)
	}
}
