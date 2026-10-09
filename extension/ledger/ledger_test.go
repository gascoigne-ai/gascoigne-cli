package ledger

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
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

func TestReopenAndPrune(t *testing.T) {
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

func TestStats(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	events := []component.Event{
		{Session: "s1", Route: "default", RawTokens: 100, OutTokens: 20, Outcome: component.Optimized, Steps: []component.Step{
			{Optimizer: "npm", Family: "js", RawTokens: 100, OutTokens: 50},
			{Optimizer: "log", Family: "data", RawTokens: 50, OutTokens: 20},
		}},
		{Session: "s1", Route: "default", RawTokens: 50, OutTokens: 50, Outcome: component.Failed},
		{Session: "s2", RawTokens: 10, OutTokens: 10, Outcome: component.Bypassed},
		{Session: "s2", Route: "default", RawTokens: 40, OutTokens: 10, Outcome: component.Optimized, Steps: []component.Step{
			{Optimizer: "bun", Family: "js", RawTokens: 40, OutTokens: 10},
		}},
	}
	for _, ev := range events {
		ev.TS = time.Now()
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		q    component.StatsQuery
		want []component.Stat
	}{
		{component.StatsQuery{}, []component.Stat{
			{Group: "default", Runs: 3, Optimized: 2, Failed: 1, RawTokens: 190, OutTokens: 80},
			{Group: "-", Runs: 1, RawTokens: 10, OutTokens: 10},
		}},
		{component.StatsQuery{By: component.BySession}, []component.Stat{
			{Group: "s1", Runs: 2, Optimized: 1, Failed: 1, RawTokens: 150, OutTokens: 70},
			{Group: "s2", Runs: 2, Optimized: 1, RawTokens: 50, OutTokens: 20},
		}},
		{component.StatsQuery{By: component.ByFamily}, []component.Stat{
			{Group: "js", Runs: 2, Optimized: 2, RawTokens: 140, OutTokens: 60},
			{Group: "data", Runs: 1, Optimized: 1, RawTokens: 50, OutTokens: 20},
		}},
		{component.StatsQuery{By: component.ByOptimizer, Session: "s1"}, []component.Stat{
			{Group: "npm", Runs: 1, Optimized: 1, RawTokens: 100, OutTokens: 50},
			{Group: "log", Runs: 1, Optimized: 1, RawTokens: 50, OutTokens: 20},
		}},
	}
	for _, tt := range tests {
		got, err := l.Stats(ctx, tt.q)
		if err != nil {
			t.Fatal(err)
		}
		for i := range got {
			if got[i].First.IsZero() || got[i].Last.Before(got[i].First) {
				t.Errorf("%+v: bad time range %v..%v", tt.q, got[i].First, got[i].Last)
			}
			got[i].First, got[i].Last = time.Time{}, time.Time{}
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%+v:\n got  %+v\n want %+v", tt.q, got, tt.want)
		}
	}
	if _, err := l.Stats(ctx, component.StatsQuery{By: "bogus"}); err == nil {
		t.Error("unknown grouping accepted")
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
