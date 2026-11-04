package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestMemoryLedgerRoundTripWritesNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	t.Setenv(EnvPath, "")
	l := start(t, &Config{Memory: true, RawTTL: time.Hour})
	ctx := t.Context()

	if l.Path() != MemoryPath {
		t.Fatalf("Path = %q, want %q", l.Path(), MemoryPath)
	}
	id, err := l.SaveRaw(ctx, "cmd:ls", payload.New(payload.CmdOutput, "a\nb\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Record(ctx, component.Event{Agent: "sdk", Session: "s1", RawTokens: 100, OutTokens: 40, RawID: id, Outcome: component.Optimized}); err != nil {
		t.Fatal(err)
	}
	// Many calls must all see the one database, not a fresh empty one per connection.
	for range 20 {
		if _, got, err := l.GetRaw(ctx, id); err != nil || got.Text() != "a\nb\n" {
			t.Fatalf("GetRaw: %v", err)
		}
	}
	stats, err := l.Stats(ctx, component.StatsQuery{By: "agent"})
	if err != nil || len(stats) != 1 || stats[0].Saved() != 60 {
		t.Fatalf("Stats = %+v, err %v", stats, err)
	}
	entries, err := l.Recent(ctx, component.StatsQuery{Session: "s1"}, 0, 10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("Recent = %+v, err %v", entries, err)
	}
	var files []string
	filepath.WalkDir(dir, func(path string, entry os.DirEntry, _ error) error {
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if len(files) != 0 {
		t.Fatalf("memory ledger wrote files: %v", files)
	}
}

func TestMemoryLedgerPathSpelling(t *testing.T) {
	t.Setenv(EnvPath, "")
	for _, cfg := range []*Config{{Path: MemoryPath}, {Memory: true}} {
		l := start(t, cfg)
		if l.Path() != MemoryPath {
			t.Fatalf("%+v: Path = %q", cfg, l.Path())
		}
	}
	t.Setenv(EnvPath, MemoryPath)
	if l := start(t, &Config{Path: "/does/not/matter.db"}); l.Path() != MemoryPath {
		t.Fatalf("env override: Path = %q", l.Path())
	}
}

func TestMemoryLedgerEvictsOldestRawOverCap(t *testing.T) {
	t.Setenv(EnvPath, "")
	l := start(t, &Config{Memory: true, MemoryMaxBytes: 4 << 10})
	ctx := t.Context()
	var ids []string
	for i := range 6 {
		// Random-looking text so gzip cannot shrink it away.
		text := strings.Repeat(string(rune('a'+i)), 1) + pseudoRandom(i, 2<<10)
		id, err := l.SaveRaw(ctx, "k", payload.New(payload.CmdOutput, text))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
		time.Sleep(2 * time.Millisecond)
	}
	if _, _, err := l.GetRaw(ctx, ids[0]); err == nil {
		t.Fatal("oldest original kept past the cap")
	}
	if _, _, err := l.GetRaw(ctx, ids[len(ids)-1]); err != nil {
		t.Fatalf("newest original evicted: %v", err)
	}
	u, err := l.RawUsage(ctx)
	if err != nil || u.Bytes > 4<<10 {
		t.Fatalf("usage %+v over the cap, err %v", u, err)
	}
}

func pseudoRandom(seed, n int) string {
	out := make([]byte, n)
	state := uint32(seed*2654435761 + 1)
	for i := range out {
		state = state*1664525 + 1013904223
		out[i] = 'a' + byte(state>>24)%26
	}
	return string(out)
}
