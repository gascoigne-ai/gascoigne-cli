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
	for _, id := range []string{"outcomes", "commands", "optimizers", "dialects", "reasons", "failures", "families", "models"} {
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
assert.match(proseFact(idle)[1], /left the text as it was/);
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
