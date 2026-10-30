package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The table helpers (sorting, filtering, the header click cycle) are plain
// functions in the page; run them under node when it is installed.
func TestDashboardTableHelpers(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed: the page's table helpers are not run")
	}
	page := string(dashboardHTML)
	cut := func(from, to string) string {
		a, b := strings.Index(page, from), strings.Index(page, to)
		if a < 0 || b < a {
			t.Fatalf("page lost %q..%q", from, to)
		}
		return page[a:b]
	}
	js := "class Node {}\nconst assert = require('assert');\n" +
		cut("// sortKey is what", "function header") +
		cut("// filterRows keeps", "// lazyTable fills") + `
const cols = [
  { title: "Cmd", get: (r) => r.n },
  { title: "Saved", get: (r) => r.s + "k", sort: (r) => r.s, num: true },
  { title: "", get: () => new Node() },
];
const rows = [{ n: "b", s: 9 }, { n: "a", s: 100 }, { n: "c", s: 10 }];
const names = (rs) => rs.map((r) => r.n).join("");
assert.strictEqual(names(sortRows(cols, rows, { i: 1, desc: true })), "acb"); // numbers, not "9k" > "100k"
assert.strictEqual(names(sortRows(cols, rows, { i: 0, desc: false })), "abc");
assert.strictEqual(names(sortRows(cols, rows, null)), "bac");
assert.strictEqual(names(filterRows(cols, rows, "A")), "a");
assert.strictEqual(names(filterRows(cols, rows, "100")), "a");
assert.strictEqual(filterRows(cols, rows, "  ").length, 3);
let s = nextSort(cols, null, 1);
assert.deepStrictEqual(s, { i: 1, desc: true });
s = nextSort(cols, s, 1);
assert.deepStrictEqual(s, { i: 1, desc: false });
assert.strictEqual(nextSort(cols, s, 1), null);
s = nextSort(cols, null, 0);
assert.deepStrictEqual(s, { i: 0, desc: false });
assert.deepStrictEqual(nextSort(cols, s, 0), { i: 0, desc: true });
`
	file := filepath.Join(t.TempDir(), "table.js")
	if err := os.WriteFile(file, []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, file).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// Every table of the page sorts and filters: summary tables go through
// lazyTable (which adds both), the recent-runs feed has its own header and
// filter box.
func TestDashboardTablesSortAndFilter(t *testing.T) {
	page := string(dashboardHTML)
	if m := regexp.MustCompile(`\$\("[a-z-]+"\)\.replaceChildren\(table\(`).FindString(page); m != "" {
		t.Errorf("a table is drawn without sorting and filtering: %s", m)
	}
	for _, id := range []string{"outcomes", "commands", "optimizers", "dialects", "reasons", "failures"} {
		if !strings.Contains(page, `lazyTable($("`+id+`")`) {
			t.Errorf("table %q is not drawn with lazyTable", id)
		}
	}
	for _, want := range []string{`id="feed-filter"`, `id="feed-head"`, "renderFeedHead()", "feedCols"} {
		if !strings.Contains(page, want) {
			t.Errorf("the recent-runs table lost %s", want)
		}
	}
}

// A run the prose model shortened shows it in the feed and in its details;
// a run where it was in the chain but changed nothing says so.
func TestDashboardShowsProse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed: the page's prose info is not run")
	}
	page := string(dashboardHTML)
	cut := func(from, to string) string {
		a, b := strings.Index(page, from), strings.Index(page, to)
		if a < 0 || b < a {
			t.Fatalf("page lost %q..%q", from, to)
		}
		return page[a:b]
	}
	js := `const assert = require('assert');
const human = (n) => String(n);
const pct = (saved, raw) => raw > 0 ? (100 * saved / raw) : 0;
const fmtPct = (p) => p.toFixed(1) + "%";
const el = (tag, attrs, ...kids) => ({ tag, attrs: attrs || {}, text: kids.flat().join("") });
` + cut("// proseStep is", "const dialectsOf") + cut("function proseTag", "function feedRow") + cut("// proseFact says", "function detailsView") + `
const ran = { optimizers: ["doc", "prose"], steps: [{ optimizer: "doc", raw_tokens: 900, out_tokens: 700 }, { optimizer: "prose", raw_tokens: 700, out_tokens: 630 }] };
const idle = { optimizers: ["doc", "prose"], steps: [{ optimizer: "doc", raw_tokens: 900, out_tokens: 700 }] };
const off = { optimizers: ["doc"], steps: [] };
assert.strictEqual(proseTag(ran).text, "prose −70");
assert.match(proseTag(ran).attrs.title, /700 → 630/);
assert.strictEqual(proseTag(idle), "");
assert.match(proseFact(ran)[1], /700 → 630 tokens \(−70, 10\.0%\)/);
assert.match(proseFact(idle)[1], /did not change this output/);
assert.strictEqual(proseFact(off), null);
`
	file := filepath.Join(t.TempDir(), "prose.js")
	if err := os.WriteFile(file, []byte(js), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, file).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{`id="experiments-panel"`, `renderExperiments(`, `/api/experiments`, `proseTag(e)`, `proseFact(e)`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lost %s", want)
		}
	}
}

// The page reads: summary first, the breakdown opening on the optimizer tab;
// experiments are a status line at the bottom, not a panel of their own.
func TestDashboardLayout(t *testing.T) {
	page := string(dashboardHTML)
	at := func(s string) int {
		i := strings.Index(page, s)
		if i < 0 {
			t.Fatalf("the page lost %s", s)
		}
		return i
	}
	if !(at(`id="kpis"`) < at(`id="feed"`) && at(`id="feed"`) < at(`id="breakdown-panel"`) && at(`id="breakdown-panel"`) < at(`id="experiments-panel"`) && at(`id="experiments-panel"`) < at(`<footer>`)) {
		t.Error("want summary, runs, breakdown, experiments status line, footer in that order")
	}
	if strings.Contains(page[at(`id="breakdown-panel"`):], `class="panel"`) {
		t.Error("something follows the breakdown as a panel of its own")
	}
	if !(at(`data-title="By optimizer"`) < at(`data-title="By command"`)) {
		t.Error("the breakdown should open on the optimizer tab")
	}
	for _, want := range []string{`icon("flask")`, `class: "chip " + x.state`, `.chip.ready::before`, `.chip.not-installed::before`} {
		at(want) // the status line draws a flask, and a chip per state
	}
	// The breakdown reads like Recent runs: a description beside a segmented control with icons.
	for _, want := range []string{`class: "feed-head"`, `class: "seg", role: "group", "aria-label": "Breakdown by"`, `icon(p.dataset.icon)`, `data-icon="sliders" onclick`} {
		at(want)
	}
	if strings.Contains(page, `class: "tabs", role: "tablist" });
  const show`) || strings.Contains(page, `>change</a>`) {
		t.Error("the breakdown tabs or the change link came back")
	}
	if strings.Contains(page, `<h2>Experiments</h2>`) {
		t.Error("experiments came back as a panel")
	}
}
