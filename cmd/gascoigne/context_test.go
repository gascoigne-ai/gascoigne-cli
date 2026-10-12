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
	return lineAt(id, "2026-10-10T10:00:00Z", model, input, read, write, output, extra)
}

func lineAt(id, ts, model string, input, read, write, output int, extra string) string {
	return fmt.Sprintf(`{"type":"assistant","sessionId":"s1","cwd":"/home/u/work/shop","timestamp":%q%s,"message":{"id":%q,"model":%q,"usage":{"input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens":%d}}}`,
		ts, extra, id, model, input, read, write, output)
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
	s, err := readSession(strings.NewReader(text), "abc.jsonl", time.Time{})
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
	s, _ := readSession(strings.NewReader(strings.Join(lines, "\n")), "x.jsonl", time.Time{})
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
	base, n := simulate(ts, 0, 0, m)
	if n != 0 || base <= 0 {
		t.Fatalf("baseline %.2f, %d compactions", base, n)
	}
	if above, n := simulate(ts, 600_000, 0, m); n != 0 || above != base {
		t.Fatalf("a limit above the peak changes the cost: %.2f vs %.2f (%d)", above, base, n)
	}
	at250, n250 := simulate(ts, 250_000, 0, m)
	at150, n150 := simulate(ts, 150_000, 0, m)
	if n250 == 0 || at250 >= base {
		t.Fatalf("compacting at 250k: %.2f vs baseline %.2f (%d)", at250, base, n250)
	}
	if n150 <= n250 || at150 >= base {
		t.Fatalf("compacting at 150k: %.2f (%d) vs 250k %.2f (%d), baseline %.2f", at150, n150, at250, n250, base)
	}
	t.Logf("baseline %.2f, 250k %.2f (%d compactions), 150k %.2f (%d): more compactions are not always cheaper", base, at250, n250, at150, n150)
	if got, _ := simulate(nil, 100_000, 0, m); got != 0 {
		t.Fatal("empty session costs money")
	}
	// A conversation that really was compacted is not compacted twice: the reset is followed.
	reset := append(growing()[:60], turn{ctx: 70_000, write: 70_000, output: 300}, turn{ctx: 75_000, read: 70_000, write: 5_000, output: 300})
	if _, n := simulate(reset, 400_000, 0, m); n != 0 {
		t.Fatalf("compacted a session that peaked at 345k then reset: %d", n)
	}
}

func TestBuildContext(t *testing.T) {
	mk := func(model string, ts []turn) sessionCost {
		s := sessionCost{ID: model, Model: model, Priced: true, turns: ts, End: time.Now()}
		s.finish()
		return s
	}
	r := buildContext([]sessionCost{mk("claude-opus-5-5", growing()), mk("local/llama", growing())}, 14*24*time.Hour)
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
	if empty := buildContext(nil, 7*24*time.Hour); len(empty.Sessions) != 0 || empty.WhatIf[0].SavedUSD != 0 {
		t.Fatalf("empty %+v", empty)
	}
}

// loadContext and the page, over transcripts in a home directory.
func TestContextEndpoint(t *testing.T) {
	home := sandbox(t)
	_, h := testDashboard(t)
	t.Run("no Claude Code here: an empty report, not an error", func(t *testing.T) {
		w := get(h, "GET", "/api/context?since=14d")
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
		w := get(h, "GET", "/api/context?since=14d")
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
		w := get(h, "GET", "/api/context?since=90d")
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
		if !strings.Contains(w.Body.String(), "Copy the full session id") {
			t.Fatal("the sessions table has no session id to copy")
		}
	})
}

func TestPrintContext(t *testing.T) {
	var b strings.Builder
	s := sessionCost{ID: "x", Project: "shop", Model: "claude-opus-5-5", Priced: true, turns: growing(), End: time.Now(), Start: time.Now()}
	s.finish()
	printContext(&b, buildContext([]sessionCost{s}, 14*24*time.Hour))
	out := b.String()
	for _, want := range []string{"CONTEXT OF A REQUEST", "over 400k", "compacted when they reached", "150k", "shop", "not detail"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	b.Reset()
	printContext(&b, buildContext(nil, 7*24*time.Hour))
	if !strings.Contains(b.String(), "No Claude Code transcripts") {
		t.Fatalf("empty: %q", b.String())
	}
}

func TestRewrites(t *testing.T) {
	at := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	mk := func(ctx, read int, after time.Duration) turn {
		return turn{ctx: ctx, read: read, write: ctx - read, ts: at.Add(after)}
	}
	ts := []turn{
		mk(100_000, 0, 0),
		mk(105_000, 100_000, time.Minute),     // warm: only growth is written
		mk(110_000, 0, 3*time.Minute),         // read nothing within the lifetime: something changed
		mk(115_000, 0, 20*time.Minute),        // paused past 5 minutes, the cache is gone
		mk(120_000, 0, 120*time.Minute),       // paused over an hour
		mk(125_000, 119_000, 121*time.Minute), // a read 1% short is not a miss
		mk(40_000, 0, 122*time.Minute),        // the context fell to a third: a compaction
		mk(50_000, 40_000, 123*time.Minute),   // warm again, growing
		mk(48_000, 0, 124*time.Minute),        // two minutes later it is smaller and nothing was read: older content removed
		mk(60_000, 10_000, 125*time.Minute),   // a fixed start was read, the rest rewritten
	}
	got := rewrites(ts)
	want := map[int]string{2: causeChanged, 3: causeFive, 4: causeHour, 6: causeCompaction, 8: causeShrank, 9: causeStart}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i, cause := range want {
		if got[i].cause != cause {
			t.Errorf("turn %d: %q, want %q", i, got[i].cause, cause)
		}
	}
	if got[2].tokens != 105_000 || got[4].tokens != 115_000 {
		t.Fatalf("tokens %+v", got)
	}
	if len(rewrites(nil)) != 0 || len(rewrites(ts[:1])) != 0 {
		t.Fatal("a session of one request rewrote something")
	}

	s := sessionCost{ID: "x", Model: "claude-opus-5-5", Priced: true, turns: ts, End: at}
	s.finish()
	r := buildContext([]sessionCost{s}, 14*24*time.Hour)
	m, _ := cost.Lookup("claude-opus-5-5")
	byCause := map[string]cacheRewrite{}
	for _, w := range r.Rewrites {
		byCause[w.Cause] = w
	}
	if byCause[causeHour].Turns != 1 || byCause[causeShrank].Turns != 1 || byCause[causeStart].Turns != 1 {
		t.Fatalf("rewrites %+v", r.Rewrites)
	}
	if d := byCause[causeHour].CostUSD - float64(115_000)*(m.CacheWrite-m.CacheRead)/1e6; d > 1e-9 || d < -1e-9 {
		t.Fatalf("extra cost %.6f", byCause[causeHour].CostUSD)
	}
}

// A session that spans days is cut to the part inside the period, so a 24-hour
// view does not charge it for what happened last week.
func TestReadSessionSince(t *testing.T) {
	var lines []string
	for i := range 6 {
		day := 10 + i/2 // two turns a day, October 10 to 12
		ts := fmt.Sprintf("2026-10-%02dT%02d:00:00Z", day, 9+i%2)
		lines = append(lines, lineAt(fmt.Sprint("m", i), ts, "claude-opus-5-5", 0, 50_000+i*10_000, 1_000, 200, ""))
	}
	text := strings.Join(lines, "\n")
	all, _ := readSession(strings.NewReader(text), "x.jsonl", time.Time{})
	cut, _ := readSession(strings.NewReader(text), "x.jsonl", time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
	if all.Turns != 6 || cut.Turns != 2 {
		t.Fatalf("turns: all %d, cut %d", all.Turns, cut.Turns)
	}
	if cut.CostUSD >= all.CostUSD || cut.Start.Day() != 12 {
		t.Fatalf("cost %.2f vs %.2f, start %v", cut.CostUSD, all.CostUSD, cut.Start)
	}
	// What a conversation restarts from is the smallest context seen, not the first turn of the cut.
	m, _ := cost.Lookup("claude-opus-5-5")
	base := []turn{{ctx: 300_000, read: 299_000, write: 1_000}, {ctx: 301_000, read: 300_000, write: 1_000}, {ctx: 302_000, read: 301_000, write: 1_000}}
	if usd, n := simulate(base, 250_000, 48_000, m); n == 0 || usd <= 0 {
		t.Fatalf("simulate over a cut session: %.2f, %d", usd, n)
	}
	// Without the true start, a session whose every context is over the limit cannot shrink: no compactions, no savings.
	allOver := []turn{{ctx: 300_000, read: 299_000, write: 1_000}, {ctx: 301_000, read: 300_000, write: 1_000}}
	b0, _ := simulate(allOver, 0, 0, m)
	if b1, n := simulate(allOver, 250_000, 0, m); n != 0 || b1 != b0 {
		t.Fatalf("compacted a session that cannot get smaller: %.2f vs %.2f, %d", b1, b0, n)
	}
	if all.floor != 51_000 || cut.floor != 51_000 {
		t.Fatalf("floor %d, cut %d: the start of the whole session, not of the period", all.floor, cut.floor)
	}
}

func TestContextEndpointPeriods(t *testing.T) {
	home := sandbox(t)
	_, h := testDashboard(t)
	dir := filepath.Join(home, ".claude", "projects", "p")
	os.MkdirAll(dir, 0o755)
	recent := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	older := time.Now().UTC().Add(-30 * time.Hour).Format(time.RFC3339)
	write := func(name, ts string, mod time.Time) {
		os.WriteFile(filepath.Join(dir, name), []byte(lineAt("m1", ts, "claude-opus-5-5", 5, 40_000, 2_000, 100, "")), 0o644)
		os.Chtimes(filepath.Join(dir, name), mod, mod)
	}
	write("recent.jsonl", recent, time.Now())
	write("older.jsonl", older, time.Now().Add(-30*time.Hour))
	count := func(since string) (int, int) {
		w := get(h, "GET", "/api/context?since="+since)
		var r contextReport
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != 200 {
			t.Fatalf("%s: %d %v: %s", since, w.Code, err, w.Body.String())
		}
		return len(r.Sessions), r.Hours
	}
	if n, hours := count("24h"); n != 1 || hours != 24 {
		t.Fatalf("24h: %d sessions, %d hours", n, hours)
	}
	if n, _ := count("7d"); n != 2 {
		t.Fatalf("7d: %d sessions", n)
	}
	if w := get(h, "GET", "/api/context?since=soon"); w.Code != 400 {
		t.Fatalf("bad since: %d", w.Code)
	}
	if got := periodLabel(24); got != "24 hours" || periodLabel(336) != "14 days" {
		t.Fatalf("labels %q %q", got, periodLabel(336))
	}
}

func TestSharedPageAssets(t *testing.T) {
	_, h := testDashboard(t)
	for url, want := range map[string]string{"/assets/pages.css": "text/css", "/assets/pages.js": "text/javascript"} {
		w := get(h, "GET", url)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), want) || w.Body.Len() < 500 {
			t.Fatalf("%s: %d %q (%d bytes)", url, w.Code, w.Header().Get("Content-Type"), w.Body.Len())
		}
	}
	for _, page := range []string{"/context", "/mcp"} {
		w := get(h, "GET", page)
		body := w.Body.String()
		if !strings.Contains(body, `/assets/pages.css`) || !strings.Contains(body, `G.header(`) {
			t.Errorf("%s does not use the shared look", page)
		}
	}
}

func TestInjectedText(t *testing.T) {
	tests := []struct {
		name, raw, label string
		want             bool
	}{
		{"a SessionStart hook is named by what it runs", `{"type":"hook_success","hookEvent":"SessionStart","command":"bash /home/u/plugins/caveman/hooks/activate.sh","content":"CAVEMAN MODE ACTIVE: respond terse"}`, "hook SessionStart: hooks/activate.sh", true},
		{"a hook that said nothing adds nothing", `{"type":"hook_success","hookEvent":"PreToolUse","command":"x","content":""}`, "", false},
		{"a prompt hook's context", `{"type":"hook_additional_context","hookEvent":"UserPromptSubmit","content":["Keep answers short."]}`, "hook UserPromptSubmit (context added, hook not named)", true},
		{"the same SessionStart text is not counted twice", `{"type":"hook_additional_context","hookEvent":"SessionStart","content":["CAVEMAN MODE ACTIVE"]}`, "", false},
		{"instruction files", `{"type":"instructions","files":[{"path":"CLAUDE.md","content":"Use tabs. Run make check."}]}`, "instruction files (CLAUDE.md, rules, memory)", true},
		{"skill listing", `{"type":"skill_listing","content":"- verify: run the checks","skillCount":1}`, "skill listing", true},
		{"bookkeeping adds nothing", `{"type":"total_tokens_reminder","content":"x"}`, "", false},
		{"not json", `nope`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			label, n := injectedText(json.RawMessage(tt.raw))
			if (n > 0) != tt.want || label != tt.label {
				t.Fatalf("got %q, %d tokens", label, n)
			}
		})
	}
}

func TestInjectedCost(t *testing.T) {
	var lines []string
	lines = append(lines, `{"type":"attachment","timestamp":"2026-10-10T10:00:00Z","attachment":{"type":"skill_listing","content":"`+strings.Repeat("skill ", 500)+`"}}`)
	for i := range 10 {
		lines = append(lines, line(fmt.Sprint("m", i), "claude-opus-5-5", 0, 40_000, 1_000, 100, ""))
		if i == 4 { // injected after five requests: five more read it
			lines = append(lines, `{"type":"attachment","timestamp":"2026-10-10T10:00:00Z","attachment":{"type":"hook_additional_context","hookEvent":"UserPromptSubmit","content":["`+strings.Repeat("be brief ", 100)+`"]}}`)
		}
	}
	s, _ := readSession(strings.NewReader(strings.Join(lines, "\n")), "x.jsonl", time.Time{})
	if len(s.inj) != 2 || s.inj[0].before != 0 || s.inj[1].before != 5 {
		t.Fatalf("injections %+v", s.inj)
	}
	r := buildContext([]sessionCost{s}, 24*time.Hour)
	m, _ := cost.Lookup("claude-opus-5-5")
	bySource := map[string]injectedSource{}
	for _, x := range r.Injected {
		bySource[x.Source] = x
	}
	skill := bySource["skill listing"]
	want := float64(skill.Tokens) * (m.CacheWrite + 10*m.CacheRead) / 1e6 // read by all ten requests
	if d := skill.CostUSD - want; d > 1e-9 || d < -1e-9 || skill.Times != 1 || skill.Sessions != 1 || skill.Rereads != skill.Tokens*10 {
		t.Fatalf("%+v, want cost %.6f", skill, want)
	}
	hook := bySource["hook UserPromptSubmit (context added, hook not named)"]
	if hook.Rereads != hook.Tokens*5 {
		t.Fatalf("a late injection is read by fewer requests: %+v", hook)
	}
	if len(r.Injected) != 2 || r.Injected[0].CostUSD < r.Injected[1].CostUSD {
		t.Fatalf("most expensive first: %+v", r.Injected)
	}
}

func TestRewriteEvidence(t *testing.T) {
	at := time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC)
	a := turn{ctx: 100_000, read: 99_000, write: 1_000, ts: at, model: "claude-opus-5-5"}
	switched := turn{ctx: 101_000, read: 0, write: 101_000, ts: at.Add(time.Minute), model: "claude-sonnet-5-5"}
	marked := turn{ctx: 102_000, read: 0, write: 102_000, ts: at.Add(2 * time.Minute), model: "claude-sonnet-5-5", marks: []string{"tool set", "skill listing"}}
	got := rewrites([]turn{a, switched, marked})
	if got[1].cause != causeModel {
		t.Errorf("a switched model: %q", got[1].cause)
	}
	if got[2].cause != causeMarked+"tool set, skill listing" {
		t.Errorf("after recorded changes: %q", got[2].cause)
	}
	for _, m := range []string{"deferred_tools_delta", "instructions", "skill_listing", "agent_listing_delta", "mcp_instructions_delta", "model"} {
		if prefixMark(json.RawMessage(`{"type":"`+m+`"}`)) == "" {
			t.Errorf("%s is not a mark", m)
		}
	}
	if prefixMark(json.RawMessage(`{"type":"hook_success"}`)) != "" || prefixMark(json.RawMessage(`x`)) != "" {
		t.Error("bookkeeping is a mark")
	}
}

func TestOneHourCacheWrites(t *testing.T) {
	opus, _ := cost.Lookup("claude-opus-5-5")
	five := turn{ctx: 100_000, write: 100_000}
	hour := turn{ctx: 100_000, write: 100_000, write1h: 100_000}
	half := turn{ctx: 100_000, write: 100_000, write1h: 50_000}
	if d := five.cost(opus) - float64(100_000)*opus.CacheWrite/1e6; d > 1e-9 || d < -1e-9 {
		t.Fatalf("5-minute write %.6f", five.cost(opus))
	}
	if d := hour.cost(opus) - float64(100_000)*2*opus.Input/1e6; d > 1e-9 || d < -1e-9 || hour.cost(opus) <= five.cost(opus) {
		t.Fatalf("1-hour write %.6f vs %.6f", hour.cost(opus), five.cost(opus))
	}
	if d := half.cost(opus) - (five.cost(opus)+hour.cost(opus))/2; d > 1e-9 || d < -1e-9 {
		t.Fatalf("a mix is the mean: %.6f", half.cost(opus))
	}
	if e := effectiveModel(opus, []turn{half}); e.CacheWrite <= opus.CacheWrite || e.CacheWrite >= 2*opus.Input {
		t.Fatalf("effective write price %.2f", e.CacheWrite)
	}
	if e := effectiveModel(opus, []turn{five}); e.CacheWrite != opus.CacheWrite {
		t.Fatal("no 1-hour writes: the model is unchanged")
	}
	line := `{"type":"assistant","timestamp":"2026-10-10T10:00:00Z","message":{"id":"a","model":"claude-opus-5-5","usage":{"input_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":1000,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":1000},"output_tokens":5}}}`
	s, _ := readSession(strings.NewReader(line), "x.jsonl", time.Time{})
	if s.turns[0].write1h != 1000 {
		t.Fatalf("the 1-hour split was not read: %+v", s.turns[0])
	}
}

// A subagent runs in a conversation of its own: its requests are not a session, but they cost money.
func TestSubagentsAreCountedApart(t *testing.T) {
	main := line("m1", "claude-opus-5-5", 5, 0, 50_000, 100, "")
	sub := line("s1", "claude-opus-5-5", 5, 20_000, 1_000, 200, `,"isSidechain":true`)
	sub2 := line("s2", "claude-opus-5-5", 5, 21_000, 1_000, 200, `,"isSidechain":true`)
	s, err := readSession(strings.NewReader(strings.Join([]string{main, sub, sub2}, "\n")), "x.jsonl", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Turns != 1 || s.subTurns != 2 || s.subUSD <= 0 {
		t.Fatalf("turns %d, subagent requests %d, $%.4f", s.Turns, s.subTurns, s.subUSD)
	}
	m, _ := cost.Lookup("claude-opus-5-5")
	want := turn{read: 20_000, write: 1_000, input: 5, output: 200}.cost(m) + turn{read: 21_000, write: 1_000, input: 5, output: 200}.cost(m)
	if d := s.subUSD - want; d > 1e-9 || d < -1e-9 {
		t.Fatalf("subagent cost %.6f, want %.6f", s.subUSD, want)
	}
	// a file that holds only a subagent is no session but its cost is in the report
	home := sandbox(t)
	dir := filepath.Join(home, ".claude", "projects", "p", "sess", "subagents")
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "agent-1.jsonl"), []byte(sub), 0o644)
	r, err := loadContext(filepath.Join(home, ".claude", "projects"), 400*24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Sessions) != 0 || r.SubagentTurns != 1 || r.SubagentUSD <= 0 {
		t.Fatalf("sessions %d, subagent %d $%.4f", len(r.Sessions), r.SubagentTurns, r.SubagentUSD)
	}
}
