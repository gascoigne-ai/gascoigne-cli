package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/cost"
)

// line is one assistant line of a transcript.
func line(id, model string, input, read, write, output int, extra string) string {
	return fmt.Sprintf(`{"type":"assistant","sessionId":"s1","cwd":"/home/u/work/shop","timestamp":"2026-10-10T10:00:00Z"%s,"message":{"id":%q,"model":%q,"usage":{"input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens":%d}}}`,
		extra, id, model, input, read, write, output)
}

func TestReadSession(t *testing.T) {
	opus, _ := cost.Lookup("claude-opus-5-5")
	text := strings.Join([]string{
		`{"type":"user","message":{"content":"hi"}}`,
		line("m1", "claude-opus-5-5", 10, 0, 50_000, 100, ""),
		line("m2", "claude-opus-5-5", 10, 50_000, 3_000, 200, ""),
		line("m2", "claude-opus-5-5", 10, 50_000, 3_000, 250, ""), // the same message streamed again: the last usage counts
		line("sub", "claude-opus-5-5", 10, 900_000, 0, 50, `,"isSidechain":true`),
		line("syn", "<synthetic>", 0, 0, 0, 0, ""),
		`not json at all`,
		line("m3", "claude-opus-5-5", 10, 53_000, 1_000, 300, ""),
		line("m4", "claude-opus-5-5", 5, 0, 20_000, 100, ""), // the context fell to a third: a compaction
		"",
	}, "\n")
	s, err := readSession(strings.NewReader(text), "abc.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "abc" || s.Project != "shop" || s.Model != "claude-opus-5-5" || !s.Priced {
		t.Fatalf("%+v", s)
	}
	if s.Turns != 4 || s.Peak != 54_010 {
		t.Fatalf("turns %d, peak %d", s.Turns, s.Peak)
	}
	// 54k to 20k is a drop below 60%, but only counts from minContext (60k) on
	if s.Compactions != 0 {
		t.Fatalf("a small conversation's drop counted as a compaction: %d", s.Compactions)
	}
	want := 0.0
	for _, tn := range []turn{{input: 10, write: 50_000, output: 100}, {input: 10, read: 50_000, write: 3_000, output: 250}, {input: 10, read: 53_000, write: 1_000, output: 300}, {input: 5, write: 20_000, output: 100}} {
		want += turnCost(opus, tn.input, tn.read, tn.write, tn.output)
	}
	if diff := s.CostUSD - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost %.6f, want %.6f", s.CostUSD, want)
	}
	if len(s.Series) != 4 || s.Series[0] != 50_010 {
		t.Fatalf("series %v", s.Series)
	}
}

func TestSessionCompactionDetected(t *testing.T) {
	var lines []string
	for i, ctx := range []int{40_000, 120_000, 300_000, 310_000, 70_000, 90_000} { // 310k -> 70k
		lines = append(lines, line(fmt.Sprint("m", i), "claude-opus-5-5", 0, ctx-1000, 1000, 100, ""))
	}
	s, _ := readSession(strings.NewReader(strings.Join(lines, "\n")), "x.jsonl")
	if s.Compactions != 1 || s.Peak != 310_000 {
		t.Fatalf("compactions %d, peak %d", s.Compactions, s.Peak)
	}
}

func TestPriceOfGatewayNames(t *testing.T) {
	for _, name := range []string{"claude-opus-5-5", "anthropic/claude-sonnet-5.5", "claude-haiku-4-5-20251001"} {
		if _, ok := priceOf(name); !ok {
			t.Errorf("%q has no price", name)
		}
	}
	if _, ok := priceOf("local/llama"); ok {
		t.Error("an unknown model is priced")
	}
}

// growing is a session whose context grows 5k a turn from 50k to about 500k.
func growing() []turn {
	var ts []turn
	for ctx := 50_000; ctx <= 500_000; ctx += 5_000 {
		ts = append(ts, turn{ctx: ctx, read: ctx - 5_000, write: 5_000, output: 400})
	}
	return ts
}

func TestSimulate(t *testing.T) {
	m, _ := cost.Lookup("claude-opus-5-5")
	ts := growing()
	base, n := simulate(ts, 0, m)
	if n != 0 || base <= 0 {
		t.Fatalf("baseline %.2f, %d compactions", base, n)
	}
	if above, n := simulate(ts, 600_000, m); n != 0 || above != base {
		t.Fatalf("a limit above the peak changes the cost: %.2f vs %.2f (%d)", above, base, n)
	}
	at250, n250 := simulate(ts, 250_000, m)
	at150, n150 := simulate(ts, 150_000, m)
	if n250 == 0 || at250 >= base {
		t.Fatalf("compacting at 250k: %.2f vs baseline %.2f (%d)", at250, base, n250)
	}
	if n150 <= n250 || at150 >= base {
		t.Fatalf("compacting at 150k: %.2f (%d) vs 250k %.2f (%d), baseline %.2f", at150, n150, at250, n250, base)
	}
	t.Logf("baseline %.2f, 250k %.2f (%d compactions), 150k %.2f (%d): more compactions are not always cheaper", base, at250, n250, at150, n150)
	if got, _ := simulate(nil, 100_000, m); got != 0 {
		t.Fatal("empty session costs money")
	}
	// A conversation that really was compacted is not compacted twice: the reset is followed.
	reset := append(growing()[:60], turn{ctx: 70_000, write: 70_000, output: 300}, turn{ctx: 75_000, read: 70_000, write: 5_000, output: 300})
	if _, n := simulate(reset, 400_000, m); n != 0 {
		t.Fatalf("compacted a session that peaked at 345k then reset: %d", n)
	}
}

func TestBuildContext(t *testing.T) {
	mk := func(model string, ts []turn) sessionCost {
		s := sessionCost{ID: model, Model: model, Priced: true, turns: ts, End: time.Now()}
		s.finish()
		return s
	}
	r := buildContext([]sessionCost{mk("claude-opus-5-5", growing()), mk("local/llama", growing())}, 14)
	if r.Unpriced != 1 || r.Turns != 2*len(growing()) {
		t.Fatalf("unpriced %d, turns %d", r.Unpriced, r.Turns)
	}
	sum, share := 0.0, 0.0
	for _, b := range r.Buckets {
		sum += b.CostUSD
		share += b.Share
	}
	if d := sum - r.CostUSD; d > 1e-6 || d < -1e-6 || share < 0.999 || share > 1.001 {
		t.Fatalf("buckets sum %.4f of %.4f, shares %.3f", sum, r.CostUSD, share)
	}
	if r.Buckets[3].Turns == 0 || r.Buckets[0].Turns == 0 {
		t.Fatalf("buckets %+v", r.Buckets)
	}
	if len(r.WhatIf) != len(thresholds) || r.WhatIf[0].SavedUSD <= 0 || r.WhatIf[1].SavedUSD <= 0 || r.WhatIf[0].SavedPct <= 0 {
		t.Fatalf("what-if %+v", r.WhatIf)
	}
	if len(r.Assumptions) < 3 || !strings.Contains(strings.Join(r.Assumptions, " "), "detail") {
		t.Fatalf("assumptions %v", r.Assumptions)
	}
	if empty := buildContext(nil, 7); len(empty.Sessions) != 0 || empty.WhatIf[0].SavedUSD != 0 {
		t.Fatalf("empty %+v", empty)
	}
}

// loadContext and the page, over transcripts in a home directory.
func TestContextEndpoint(t *testing.T) {
	home := sandbox(t)
	_, h := testDashboard(t)
	t.Run("no Claude Code here: an empty report, not an error", func(t *testing.T) {
		w := get(h, "GET", "/api/context?days=14")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"sessions":[]`) {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	})
	dir := filepath.Join(home, ".claude", "projects", "-work-shop")
	os.MkdirAll(dir, 0o755)
	var lines []string
	for i := range 30 {
		lines = append(lines, line(fmt.Sprint("m", i), "claude-opus-5-5", 5, 40_000+i*5_000, 5_000, 300, ""))
	}
	os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte(strings.Join(lines, "\n")), 0o644)
	os.WriteFile(filepath.Join(dir, "old.jsonl"), []byte(strings.Join(lines, "\n")), 0o644)
	old := time.Now().AddDate(0, 0, -40)
	os.Chtimes(filepath.Join(dir, "old.jsonl"), old, old)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a transcript"), 0o644)

	t.Run("reads recent transcripts only", func(t *testing.T) {
		w := get(h, "GET", "/api/context?days=14")
		var r contextReport
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != 200 {
			t.Fatalf("%d %v: %s", w.Code, err, w.Body.String())
		}
		if len(r.Sessions) != 1 || r.Sessions[0].Turns != 30 || r.Sessions[0].Project != "shop" || r.Turns != 30 {
			t.Fatalf("%+v", r.Sessions)
		}
		if body := w.Body.String(); strings.Contains(body, "null") {
			t.Fatalf("null in JSON for a page that reads lengths: %s", body)
		}
	})
	t.Run("a wider period reads the old one too", func(t *testing.T) {
		w := get(h, "GET", "/api/context?days=90")
		var r contextReport
		json.Unmarshal(w.Body.Bytes(), &r)
		if len(r.Sessions) != 2 {
			t.Fatalf("%d sessions", len(r.Sessions))
		}
	})
	t.Run("page", func(t *testing.T) {
		w := get(h, "GET", "/context")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>gascoigne · Context") {
			t.Fatalf("%d", w.Code)
		}
	})
}

func TestPrintContext(t *testing.T) {
	var b strings.Builder
	s := sessionCost{ID: "x", Project: "shop", Model: "claude-opus-5-5", Priced: true, turns: growing(), End: time.Now(), Start: time.Now()}
	s.finish()
	printContext(&b, buildContext([]sessionCost{s}, 14))
	out := b.String()
	for _, want := range []string{"CONTEXT OF A REQUEST", "over 400k", "compacted when they reached", "150k", "shop", "not detail"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	b.Reset()
	printContext(&b, buildContext(nil, 7))
	if !strings.Contains(b.String(), "No Claude Code transcripts") {
		t.Fatalf("empty: %q", b.String())
	}
}
