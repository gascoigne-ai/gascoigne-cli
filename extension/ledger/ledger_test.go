package ledger

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

func start(t *testing.T, cfg *Config) *Ledger {
	t.Helper()
	l := New(cfg)
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Shutdown(context.Background()) })
	return l
}

func TestRawRoundTrip(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	ctx := t.Context()
	p := payload.New(payload.CmdOutput, "line1\n\tline2\x00")
	p.Set("cmd", "ls")

	id, err := l.SaveRaw(ctx, "cmd:ls", p)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := l.SaveRaw(ctx, "cmd:ls", p); again != id {
		t.Fatalf("id not content-derived: %s != %s", again, id)
	}
	key, got, err := l.GetRaw(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if key != "cmd:ls" || got.Text() != p.Text() || got.Attrs["cmd"] != "ls" {
		t.Fatalf("GetRaw = %q %+v", key, got)
	}
	if _, _, err := l.GetRaw(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestMigrateIdempotentAndPrune(t *testing.T) {
	path := filepath.Join(t.TempDir(), "l.db")
	l := start(t, &Config{Path: path})
	id, err := l.SaveRaw(t.Context(), "", payload.New(payload.Message, "x"))
	if err != nil {
		t.Fatal(err)
	}
	l.Shutdown(t.Context())

	time.Sleep(5 * time.Millisecond)
	l = start(t, &Config{Path: path, RawTTL: time.Millisecond})
	if _, _, err := l.GetRaw(t.Context(), id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("raw not pruned: %v", err)
	}
}

func TestFailures(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	for range 2 {
		if err := l.MarkFailure(ctx, "cmd:ls", component.ReasonError); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := l.Failures(ctx, "cmd:ls"); n != 2 {
		t.Fatalf("Failures = %d, want 2", n)
	}
	fs, err := l.FailureList(ctx)
	if err != nil || len(fs) != 1 || fs[0].Reason != component.ReasonError {
		t.Fatalf("FailureList = %+v, %v", fs, err)
	}
	if err := l.Forget(ctx, "cmd:ls"); err != nil {
		t.Fatal(err)
	}
	if n, _ := l.Failures(ctx, "cmd:ls"); n != 0 {
		t.Fatalf("Failures after Forget = %d", n)
	}
}

func TestGain(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	events := []component.Event{
		{Route: "git", RawTokens: 100, OutTokens: 20, Outcome: component.Optimized},
		{Route: "git", RawTokens: 50, OutTokens: 50, Outcome: component.Failed},
		{RawTokens: 10, OutTokens: 10, Outcome: component.Bypassed},
	}
	for _, ev := range events {
		ev.TS = time.Now()
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	stats, err := l.Gain(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []Stat{
		{Route: "git", Runs: 2, Optimized: 1, Failed: 1, RawTokens: 150, OutTokens: 70},
		{Route: "-", Runs: 1, RawTokens: 10, OutTokens: 10},
	}
	if len(stats) != len(want) || stats[0] != want[0] || stats[1] != want[1] {
		t.Fatalf("Gain = %+v, want %+v", stats, want)
	}
}

func TestEnvPathOverrides(t *testing.T) {
	want := filepath.Join(t.TempDir(), "env.db")
	t.Setenv(EnvPath, want)
	got, err := (&Config{Path: "/nope.db"}).path()
	if err != nil || got != want {
		t.Fatalf("path = %q, %v", got, err)
	}
}
