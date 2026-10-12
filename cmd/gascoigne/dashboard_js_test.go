package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
