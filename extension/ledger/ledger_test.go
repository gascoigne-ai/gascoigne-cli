package ledger

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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

func TestRawStoredCompressed(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	text := strings.Repeat("ok  \tgithub.com/example/pkg\t0.012s\n", 200)
	id, err := l.SaveRaw(ctx, "", payload.New(payload.CmdOutput, text))
	if err != nil {
		t.Fatal(err)
	}
	u, err := l.RawUsage(ctx)
	if err != nil || u.Count != 1 || u.Bytes == 0 || u.Bytes > int64(len(text))/10 {
		t.Fatalf("usage %+v (text %d bytes), err %v", u, len(text), err)
	}
	if _, got, err := l.GetRaw(ctx, id); err != nil || got.Text() != text {
		t.Fatalf("round trip: err %v", err)
	}
}

func TestRawReadsUncompressedRows(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	// A row as saved before originals were compressed.
	if err := l.db.Create(&raw{ID: "old", Key: "cmd:ls", Payload: []byte(`{"blocks":[{"kind":"cmd_output","text":"a\nb"}]}`)}).Error; err != nil {
		t.Fatal(err)
	}
	if _, got, err := l.GetRaw(t.Context(), "old"); err != nil || got.Text() != "a\nb" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestFlushRaw(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	old, _ := l.SaveRaw(ctx, "", payload.New(payload.Message, "old"))
	l.db.Model(&raw{}).Where("id = ?", old).Update("created", time.Now().Add(-48*time.Hour).UnixMilli())
	fresh, _ := l.SaveRaw(ctx, "", payload.New(payload.Message, "fresh"))

	if n, err := l.FlushRaw(ctx, time.Now().Add(-24*time.Hour)); err != nil || n != 1 {
		t.Fatalf("flush older: n %d, err %v", n, err)
	}
	if _, _, err := l.GetRaw(ctx, old); !errors.Is(err, ErrNotFound) {
		t.Error("old original kept")
	}
	if _, _, err := l.GetRaw(ctx, fresh); err != nil {
		t.Error("fresh original flushed")
	}
	if n, err := l.FlushRaw(ctx, time.Time{}); err != nil || n != 1 {
		t.Fatalf("flush all: n %d, err %v", n, err)
	}
	if u, _ := l.RawUsage(ctx); u.Count != 0 {
		t.Errorf("left %+v", u)
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
	if f, _ := l.Failures(ctx, "cmd:ls"); f.Count != 2 || f.Reason != component.ReasonError || f.Last.IsZero() {
		t.Fatalf("Failures = %+v, want 2× error", f)
	}
	fs, err := l.FailureList(ctx)
	if err != nil || len(fs) != 1 || fs[0].Reason != component.ReasonError {
		t.Fatalf("FailureList = %+v, %v", fs, err)
	}
	if err := l.Forget(ctx, "cmd:ls"); err != nil {
		t.Fatal(err)
	}
	if f, _ := l.Failures(ctx, "cmd:ls"); f.Count != 0 {
		t.Fatalf("Failures after Forget = %+v", f)
	}
}

func TestStats(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	events := []component.Event{
		{Session: "s1", Command: "npm install", Route: "default", RawTokens: 100, OutTokens: 20, Outcome: component.Optimized, Steps: []component.Step{
			{Optimizer: "npm", Family: "js", Dialect: "pkg/npm", RawTokens: 100, OutTokens: 50},
			{Optimizer: "log", Family: "data", Dialect: "log/app", RawTokens: 50, OutTokens: 20},
		}},
		{Session: "s1", Command: "npm install", Route: "default", RawTokens: 50, OutTokens: 50, Outcome: component.Failed, Filter: "npm", Reason: "npm: boom"},
		{Session: "s2", Command: "ls", RawTokens: 10, OutTokens: 10, Outcome: component.Bypassed, Filter: "minsize", Reason: "10 tokens < min_tokens 200"},
		{Session: "s2", Command: "bun test", Route: "default", RawTokens: 40, OutTokens: 10, Outcome: component.Optimized, Steps: []component.Step{
			{Optimizer: "bun", Family: "js", Dialect: "pkg/bun", RawTokens: 40, OutTokens: 10},
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
		{component.StatsQuery{By: component.ByCommand}, []component.Stat{
			{Group: "npm install", Runs: 2, Optimized: 1, Failed: 1, RawTokens: 150, OutTokens: 70},
			{Group: "bun test", Runs: 1, Optimized: 1, RawTokens: 40, OutTokens: 10},
			{Group: "ls", Runs: 1, RawTokens: 10, OutTokens: 10},
		}},
		{component.StatsQuery{By: component.ByOutcome}, []component.Stat{
			{Group: "optimized", Runs: 2, Optimized: 2, RawTokens: 140, OutTokens: 30},
			{Group: "bypassed", Runs: 1, RawTokens: 10, OutTokens: 10},
			{Group: "error", Runs: 1, Failed: 1, RawTokens: 50, OutTokens: 50},
		}},
		{component.StatsQuery{By: component.ByReason}, []component.Stat{
			{Group: "npm", Command: "npm install", Outcome: "error", Reason: "npm: boom", Runs: 1, Failed: 1, RawTokens: 50, OutTokens: 50},
			{Group: "minsize", Command: "ls", Outcome: "bypassed", Reason: "10 tokens < min_tokens 200", Runs: 1, RawTokens: 10, OutTokens: 10},
		}},
		{component.StatsQuery{By: component.ByFamily}, []component.Stat{
			{Group: "js", Family: "js", Runs: 2, Optimized: 2, RawTokens: 140, OutTokens: 60},
			{Group: "data", Family: "data", Runs: 1, Optimized: 1, RawTokens: 50, OutTokens: 20},
		}},
		{component.StatsQuery{By: component.ByDialect}, []component.Stat{
			{Group: "pkg/npm", Family: "js", Runs: 1, Optimized: 1, RawTokens: 100, OutTokens: 50},
			{Group: "log/app", Family: "data", Runs: 1, Optimized: 1, RawTokens: 50, OutTokens: 20}, // ties sort by name
			{Group: "pkg/bun", Family: "js", Runs: 1, Optimized: 1, RawTokens: 40, OutTokens: 10},
		}},
		{component.StatsQuery{By: component.ByOptimizer, Session: "s1"}, []component.Stat{
			{Group: "npm", Family: "js", Runs: 1, Optimized: 1, RawTokens: 100, OutTokens: 50},
			{Group: "log", Family: "data", Runs: 1, Optimized: 1, RawTokens: 50, OutTokens: 20},
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
	got, err := (&Config{Path: "/nope.db"}).File()
	if err != nil || got != want {
		t.Fatalf("path = %q, %v", got, err)
	}
}

func TestStatsDimensionsAndFilters(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	now := time.Now()
	day := 24 * time.Hour
	for _, ev := range []component.Event{
		{TS: now, Agent: "claude-code", Model: "opus", RawTokens: 100, OutTokens: 10, Outcome: component.Optimized},
		{TS: now.Add(-2 * day), Agent: "claude-code", Model: "opus", RawTokens: 50, OutTokens: 25, Outcome: component.Optimized},
		{TS: now.Add(-40 * day), Agent: "codex", Model: "gpt", RawTokens: 80, OutTokens: 20, Outcome: component.Optimized},
	} {
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	groups := func(q component.StatsQuery) []string {
		t.Helper()
		stats, err := l.Stats(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, s := range stats {
			out = append(out, fmt.Sprintf("%s:%d", s.Group, s.Saved()))
		}
		return out
	}
	dayKey := func(t time.Time) string { return t.Format(time.DateOnly) }
	tests := []struct {
		name string
		q    component.StatsQuery
		want []string
	}{
		{"agent", component.StatsQuery{By: component.ByAgent}, []string{"claude-code:115", "codex:60"}},
		{"model filtered", component.StatsQuery{By: component.ByModel, Agent: "codex"}, []string{"gpt:60"}},
		{"since", component.StatsQuery{By: component.ByAgent, Since: now.Add(-7 * day)}, []string{"claude-code:115"}},
		{"day is chronological", component.StatsQuery{By: component.ByDay}, []string{
			dayKey(now.Add(-40*day)) + ":60", dayKey(now.Add(-2*day)) + ":25", dayKey(now) + ":90"}},
		{"month", component.StatsQuery{By: component.ByMonth, Since: now.Add(-time.Hour)}, []string{now.Format("2006-01") + ":90"}},
	}
	for _, tt := range tests {
		if got := groups(tt.q); !slices.Equal(got, tt.want) {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRecent(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	now := time.Now()
	for i, s := range []string{"a", "b", "a"} {
		ev := component.Event{TS: now.Add(time.Duration(i) * time.Second), Session: s, Command: fmt.Sprint("cmd", i),
			Outcome: component.Optimized, RawTokens: 100, OutTokens: 10, RawID: fmt.Sprint("raw", i),
			Steps: []component.Step{{Optimizer: "git", Family: "vcs", Dialect: "vcs/git-log", RawTokens: 100, OutTokens: 10}}}
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	got, err := l.Recent(ctx, component.StatsQuery{}, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Command != "cmd2" || got[1].Command != "cmd1" {
		t.Fatalf("want the 2 newest, newest first: %+v", got)
	}
	if got[0].RawID != "raw2" || len(got[0].Steps) != 1 || got[0].Steps[0].Dialect != "vcs/git-log" {
		t.Fatalf("fields or steps missing: %+v", got[0])
	}
	newer, err := l.Recent(ctx, component.StatsQuery{Session: "a"}, got[1].ID-1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(newer) != 1 || newer[0].Command != "cmd2" {
		t.Fatalf("after id and session filter: %+v", newer)
	}
}

func TestReset(t *testing.T) {
	ctx := t.Context()
	setup := func(t *testing.T) *Ledger {
		l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
		for _, s := range []string{"a", "b", "b"} {
			ev := component.Event{TS: time.Now(), Session: s, Outcome: component.Optimized, RawTokens: 10, OutTokens: 1,
				Steps: []component.Step{{Optimizer: "git", Family: "vcs", RawTokens: 10, OutTokens: 1}}}
			if err := l.Record(ctx, ev); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := l.SaveRaw(ctx, "cmd:ls", payload.New(payload.CmdOutput, "x")); err != nil {
			t.Fatal(err)
		}
		if err := l.MarkFailure(ctx, "cmd:ls", "manual"); err != nil {
			t.Fatal(err)
		}
		return l
	}
	t.Run("one session keeps the rest", func(t *testing.T) {
		l := setup(t)
		n, err := l.Reset(ctx, component.StatsQuery{Session: "b"}, false)
		if err != nil || n != 2 {
			t.Fatalf("deleted %d, %v", n, err)
		}
		left, _ := l.Recent(ctx, component.StatsQuery{}, 0, 10)
		if len(left) != 1 || left[0].Session != "a" || len(left[0].Steps) != 1 {
			t.Fatalf("left: %+v", left)
		}
		if f, _ := l.Failures(ctx, "cmd:ls"); f.Count != 1 {
			t.Fatal("failure history must survive a stats reset")
		}
	})
	t.Run("everything", func(t *testing.T) {
		l := setup(t)
		n, err := l.Reset(ctx, component.StatsQuery{}, true)
		if err != nil || n != 3 {
			t.Fatalf("deleted %d, %v", n, err)
		}
		var steps int64
		l.db.Model(&step{}).Count(&steps)
		if steps != 0 {
			t.Fatalf("%d orphan steps", steps)
		}
		if f, _ := l.Failures(ctx, "cmd:ls"); f.Count != 0 {
			t.Fatal("failures kept")
		}
		var raws int64
		l.db.Model(&raw{}).Count(&raws)
		if raws != 0 {
			t.Fatal("originals kept")
		}
	})
	t.Run("everything refuses filters", func(t *testing.T) {
		l := setup(t)
		if _, err := l.Reset(ctx, component.StatsQuery{Session: "a"}, true); err == nil {
			t.Fatal("originals and failures have no session: a filtered full reset would be a lie")
		}
	})
}

func TestPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "l.db")
	if got := start(t, &Config{Path: p}).Path(); got != p {
		t.Fatalf("Path() = %q", got)
	}
}

func TestOutputKeptWithOriginal(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	id, err := l.SaveRaw(ctx, "cmd:x", payload.New(payload.CmdOutput, "long original\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := l.GetOutput(ctx, id); ok {
		t.Fatal("output before any run")
	}
	if err := l.Record(ctx, component.Event{TS: time.Now(), RawID: id, Outcome: component.Optimized, Output: "short\n"}); err != nil {
		t.Fatal(err)
	}
	if out, ok, err := l.GetOutput(ctx, id); err != nil || !ok || out != "short\n" {
		t.Fatalf("got %q %v %v", out, ok, err)
	}
	if _, ok, err := l.GetOutput(ctx, "unknown"); ok || err != nil {
		t.Fatalf("unknown id: %v %v", ok, err)
	}
}

func TestPage(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	for i, o := range []component.Outcome{component.Optimized, component.Bypassed, component.Optimized, component.Unchanged} {
		if err := l.Record(ctx, component.Event{TS: time.Now(), Key: fmt.Sprint(i), Outcome: o}); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := l.Page(ctx, component.StatsQuery{}, Page{Limit: 10})
	opt, _ := l.Page(ctx, component.StatsQuery{}, Page{Outcomes: []string{"optimized"}, Limit: 10})
	older, _ := l.Page(ctx, component.StatsQuery{}, Page{Before: all[1].ID, Limit: 10})
	if len(all) != 4 || len(opt) != 2 || len(older) != 2 || older[0].ID != all[2].ID {
		t.Fatalf("all %d, optimized %d, older %v", len(all), len(opt), older)
	}
}

const rawReq = "raw requested: attr raw=true (-raw, MCP raw)"

func recordRuns(t *testing.T, l *Ledger, runs ...component.Event) {
	t.Helper()
	ts := time.Now().Add(-time.Minute)
	for _, e := range runs {
		ts = ts.Add(time.Millisecond)
		e.TS, e.Session = ts, "s"
		if err := l.Record(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
}

func pageKeys(t *testing.T, l *Ledger, p Page) []string {
	t.Helper()
	p.Limit = 10
	got, err := l.Page(t.Context(), component.StatsQuery{}, p)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range got {
		out = append(out, e.Key+":"+string(e.Outcome))
	}
	return out
}

func TestRerequestedOutcome(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	opt, byp := component.Optimized, component.Bypassed
	recordRuns(t, l,
		component.Event{Key: "direct", Outcome: byp, Reason: rawReq}, // asked raw at once
		component.Event{Key: "rerun", Outcome: opt},
		component.Event{Key: "rerun", Outcome: byp, Reason: rawReq}, // reran raw after an optimized run
		component.Event{Key: "fetched", Outcome: opt},
		component.Event{Key: "fetched", Outcome: opt},
		component.Event{Key: "other", Outcome: byp, Reason: "45 tokens < min_tokens 200"},
	)
	// The agent fetched the original of the latest "fetched" run.
	if err := l.MarkFailure(t.Context(), "fetched", component.ReasonRawRequested); err != nil {
		t.Fatal(err)
	}
	if got, want := pageKeys(t, l, Page{Outcomes: []string{"rerequested"}}), []string{"fetched:rerequested", "rerun:rerequested"}; !slices.Equal(got, want) {
		t.Errorf("rerequested: %v, want %v", got, want)
	}
	if got, want := pageKeys(t, l, Page{RawAsked: true}), []string{"direct:bypassed"}; !slices.Equal(got, want) {
		t.Errorf("raw: %v, want %v", got, want)
	}
	if got := pageKeys(t, l, Page{Outcomes: []string{"optimized"}}); !slices.Equal(got, []string{"fetched:optimized"}) {
		t.Errorf("optimized: %v", got)
	}
}

func TestMigrateRerequested(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	opt, byp := component.Optimized, component.Bypassed
	recordRuns(t, l,
		component.Event{Key: "rerun", Outcome: opt},
		component.Event{Key: "rerun", Outcome: byp, Reason: rawReq},
		component.Event{Key: "fetched", Outcome: opt},
		component.Event{Key: "fetched", Outcome: opt},
		component.Event{Key: "plain", Outcome: opt},
	)
	// State of a database from before the outcome existed.
	if err := l.db.Exec("UPDATE events SET outcome = 'optimized' WHERE outcome = 'rerequested'").Error; err != nil {
		t.Fatal(err)
	}
	if err := l.db.Create(&failure{Key: "fetched", Count: 1, LastTS: time.Now().UnixMilli(), Reason: component.ReasonRawRequested}).Error; err != nil {
		t.Fatal(err)
	}
	if err := l.db.Exec("PRAGMA user_version = 1").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 { // the second run must change nothing
		if err := l.migrateRerequested(ctx); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"plain:optimized", "fetched:rerequested", "fetched:optimized", "rerun:bypassed", "rerun:rerequested"}
	if got := pageKeys(t, l, Page{}); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRecentOutputs(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	now := time.Now()
	run := func(session, key, original, output string, ago time.Duration) {
		t.Helper()
		id, err := l.SaveRaw(ctx, key, payload.New(payload.CmdOutput, original))
		if err != nil {
			t.Fatal(err)
		}
		ev := component.Event{TS: now.Add(-ago), Session: session, Key: key, RawID: id, Outcome: component.Optimized, Output: output}
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	run("s1", "cmd:go test", "old original\n", "old\n", time.Hour)
	run("s1", "cmd:go test", "first original\n", "first\n", 3*time.Minute)
	run("s1", "cmd:go test", "second original\n", "second\n", time.Minute)
	run("s2", "cmd:go test", "other session\n", "other\n", time.Minute)
	run("s1", "cmd:go vet", "other key\n", "vet\n", time.Minute)

	got, err := l.RecentOutputs(ctx, "s1", "cmd:go test", now.Add(-10*time.Minute), 5)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"second\n", "first\n"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, _ := l.RecentOutputs(ctx, "s1", "cmd:go test", now.Add(-10*time.Minute), 1); len(got) != 1 {
		t.Fatalf("limit 1: got %q", got)
	}
}

func TestRawIDShortAndUnique(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	p := payload.New(payload.CmdOutput, "original\n")
	id, err := l.SaveRaw(ctx, "cmd:x", p)
	if err != nil || len(id) != 8 {
		t.Fatalf("id %q (%v), want 8 hex", id, err)
	}
	if again, _ := l.SaveRaw(ctx, "cmd:x", p); again != id {
		t.Fatalf("same original: id %q, want %q", again, id)
	}

	// Another original already holding the next one's 8-hex prefix.
	q := payload.New(payload.CmdOutput, "other\n")
	sum := sha256.Sum256([]byte(q.Text()))
	prefix := hex.EncodeToString(sum[:])[:8]
	other, _ := pack([]byte(`{"blocks":[]}`))
	if err := l.db.Create(&raw{ID: prefix, Key: "cmd:y", Payload: other}).Error; err != nil {
		t.Fatal(err)
	}
	id2, err := l.SaveRaw(ctx, "cmd:z", q)
	if err != nil || id2 != hex.EncodeToString(sum[:])[:12] {
		t.Fatalf("collision: id %q (%v), want the 12-hex prefix", id2, err)
	}
	if _, got, err := l.GetRaw(ctx, id2); err != nil || got.Text() != "other\n" {
		t.Fatalf("GetRaw(%s) = %v, %v", id2, got, err)
	}
}

func TestRawSameTextStoredOnce(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	a := payload.New(payload.CmdOutput, "same output\n")
	a.Set(payload.AttrSession, "s1")
	b := payload.New(payload.CmdOutput, "same output\n")
	b.Set(payload.AttrSession, "s2")
	ida, _ := l.SaveRaw(ctx, "cmd:x", a)
	idb, _ := l.SaveRaw(ctx, "cmd:x", b)
	var n int64
	l.db.Model(&raw{}).Count(&n)
	if ida != idb || n != 1 {
		t.Fatalf("ids %s %s, %d rows; want one", ida, idb, n)
	}
}

func TestLongKeys(t *testing.T) {
	long := "cmd:" + strings.Repeat("echo heredoc line; ", 40)
	if k := storedKey(long); len(k) > maxKey || !strings.HasPrefix(k, long[:keyPrefix]) || k == storedKey(long+"x") {
		t.Fatalf("storedKey: %q", k)
	}
	if storedKey("cmd:go test") != "cmd:go test" {
		t.Fatal("short key changed")
	}
	if k := storedKey("cmd:" + strings.Repeat("ż", 120)); !utf8.ValidString(k) {
		t.Fatalf("cut inside a rune: %q", k)
	}

	path := filepath.Join(t.TempDir(), "l.db")
	l := start(t, &Config{Path: path})
	ctx := t.Context()
	// A ledger written before keys were shortened.
	l.db.Exec("PRAGMA user_version = 0")
	l.db.Create(&event{TS: 1, Pipeline: "cli", Key: long, Route: "r", Optimizers: "", Outcome: "optimized"})
	l.db.Create(&failure{Key: long, Count: 2})
	if err := l.migrateKeys(ctx); err != nil {
		t.Fatal(err)
	}
	var keys []string
	l.db.Model(&event{}).Pluck("key", &keys)
	if len(keys) != 1 || keys[0] != storedKey(long) {
		t.Fatalf("event keys %q", keys)
	}
	if f, err := l.Failures(ctx, long); err != nil || f.Count != 2 {
		t.Fatalf("failures by full key: %+v %v", f, err)
	}
	if err := l.MarkFailure(ctx, long, "x"); err != nil {
		t.Fatal(err)
	}
	if f, _ := l.Failures(ctx, long); f.Count != 3 {
		t.Fatalf("count %d after mark", f.Count)
	}
}

func TestLedgerFilePrivate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "l.db")
	start(t, &Config{Path: path})
	for p, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, %v; want %v", p, fi.Mode().Perm(), err, want)
		}
	}
}
