package ledger

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

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
		if err := l.RecordToolList(ctx, "claude", "github", sizes); err != nil {
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
			{"2026-10-01", "claude", "github", "create_issue", 340, 0},
			{"2026-10-01", "claude", "github", "search", 125, 5}, // size of the last listing, both calls kept
			{"2026-10-02", "claude", "github", "create_issue", 340, 0},
			{"2026-10-02", "claude", "github", "search", 125, 0},
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

func TestStatsTTLValidate(t *testing.T) {
	if err := (&Config{StatsTTL: -time.Hour}).Validate(); err == nil {
		t.Fatal("negative stats_ttl accepted")
	}
	if d := NewFactory().DefaultConfig().(*Config).StatsTTL; d != DefaultStatsTTL {
		t.Fatalf("default %v", d)
	}
}
