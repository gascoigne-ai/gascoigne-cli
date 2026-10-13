package jsonoptimizer

import (
	"encoding/json"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/compress/jsonx"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func testConfig(edit func(*Config)) *Config {
	c := jsonx.Default()
	c.MaxString = 40
	c.MinTokens = 0
	if edit != nil {
		edit(c)
	}
	return c
}

func run(t *testing.T, cfg *Config, typ, in string, attrs ...string) string {
	t.Helper()
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: typ}}, Attrs: map[string]string{}}
	for i := 0; i+1 < len(attrs); i += 2 {
		p.Set(attrs[i], attrs[i+1])
	}
	if err := (&Optimizer{cfg: cfg}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(p.Blocks[0].Text, "// "+jsonx.Legend+"\n")
}

func TestDropEmptyAndStrings(t *testing.T) {
	in := `{"a": null, "b": "", "c": [], "d": {}, "e": 0, "f": false, "long": "` + strings.Repeat("x", 100) +
		`", "sha": "sha512-Aup7aUOfp1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJ"}`
	got := run(t, testConfig(nil), detect.JSON, in)
	want := `{"e":0,"f":false,"long":"xxxxxxxxxxxxxxxxxxxx … xxxxxxxxxxxxxxxxxxxx","sha":"sha512-Aup7aUOfp…"}` + "\n"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestTableWithSameAndAffix(t *testing.T) {
	in := `[{"name":"a","owner":"go","url":"https://x.io/repos/a/hooks"},
	         {"name":"b","owner":"go","url":"https://x.io/repos/b/hooks"},
	         {"name":"c","owner":"go","url":"https://x.io/repos/c/hooks"}]`
	got := run(t, testConfig(func(c *Config) { c.LosslessMinSaving = 0 }), detect.JSON, in)
	want := `{"_affix":{"url":["https://x.io/repos/","/hooks"]},"_cols":["name","url"],"_rows":[["a","a"],["b","b"],["c","c"]],"_same":{"owner":"go"}}` + "\n"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestTableNeedsMinSaving(t *testing.T) {
	in := `[{"a":"x1"},{"a":"y2"},{"a":"z3"}]`
	if got := run(t, testConfig(func(c *Config) { c.LosslessMinSaving = 0.9 }), detect.JSON, in); got != `[{"a":"x1"},{"a":"y2"},{"a":"z3"}]`+"\n" {
		t.Fatalf("table kept despite small saving: %s", got)
	}
}

func items(n int, f func(i int) map[string]any) string {
	var out []map[string]any
	for i := range n {
		out = append(out, f(i))
	}
	b, _ := json.Marshal(out)
	return string(b)
}

func TestLosslessKeepsEveryItem(t *testing.T) {
	in := items(40, func(i int) map[string]any { return map[string]any{"id": i, "kind": "widget", "price": 100 + i} })
	got := run(t, testConfig(func(c *Config) { c.LosslessMaxTokens = 10000 }), detect.JSON, in)
	if strings.Contains(got, "omitted") || !strings.Contains(got, "[39,139]") {
		t.Fatalf("lossless dropped items: %s", got)
	}
}

func TestSampleSignals(t *testing.T) {
	in := items(200, func(i int) map[string]any {
		m := map[string]any{"i": i, "level": "info", "dur": 10 + i%7, "state": "ok", "path": fmt.Sprintf("/api/items/%d", i)}
		switch i {
		case 60:
			m["path"] = "/api/checkout"
		case 77:
			m["level"] = "error"
		case 120:
			m["dur"] = 5000
		case 150:
			m["retry_after"] = 30
		}
		if i >= 100 {
			m["state"] = "degraded"
		}
		return m
	})
	got := run(t, testConfig(func(c *Config) { c.LosslessMaxTokens = 0 }), detect.JSON, in, payload.AttrQuery, "checkout")
	for name, want := range map[string]string{
		"head":         `,0,`,
		"tail":         `,199,`,
		"query":        `/api/checkout`,
		"rare value":   `"error"`,
		"anomaly":      `5000`,
		"rare key":     `,30]`,
		"change point": `,100,`,
		"marker":       `items omitted`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%s: missing %s in %s", name, want, got)
		}
	}
	var table struct {
		Rows []any `json:"_rows"`
	}
	json.Unmarshal([]byte(got), &table)
	kept := 0
	for _, r := range table.Rows {
		if _, ok := r.([]any); ok {
			kept++
		}
	}
	if kept > 17 {
		t.Errorf("kept %d rows, want about max_items (15)", kept)
	}
}

func TestDedupe(t *testing.T) {
	in := items(51, func(i int) map[string]any {
		if i == 25 {
			return map[string]any{"msg": "different"}
		}
		return map[string]any{"msg": "same"}
	})
	got := run(t, testConfig(func(c *Config) { c.LosslessMaxTokens = 0 }), detect.JSON, in)
	if !strings.Contains(got, "… 49 identical items omitted") || !strings.Contains(got, "different") {
		t.Fatalf("got %s", got)
	}
}

func TestStreamAndNDJSONBecomeTables(t *testing.T) {
	cfg := testConfig(func(c *Config) { c.LosslessMinSaving = 0 })
	for _, typ := range []string{detect.JSON, detect.NDJSON} {
		got := run(t, cfg, typ, "{\"alpha\":1,\"bravo\":2}\n{\"alpha\":3,\"bravo\":4}\n{\"alpha\":5,\"bravo\":6}\n")
		if want := `{"_cols":["alpha","bravo"],"_rows":[[1,2],[3,4],[5,6]]}` + "\n"; got != want {
			t.Errorf("%s: got %s", typ, got)
		}
	}
}

func TestDeterministic(t *testing.T) {
	in := items(300, func(i int) map[string]any {
		return map[string]any{"i": i, "v": (i * 7919) % 101, "s": []string{"a", "b"}[i%2]}
	})
	cfg := testConfig(func(c *Config) { c.LosslessMaxTokens = 0 })
	if a, b := run(t, cfg, detect.JSON, in), run(t, cfg, detect.JSON, in); a != b {
		t.Fatalf("nondeterministic:\n%s\n%s", a, b)
	}
}

func TestInvalidJSONErrors(t *testing.T) {
	p := &payload.Payload{Blocks: []payload.Block{{Text: `{"a":`, Type: detect.JSON}}}
	if err := (&Optimizer{cfg: testConfig(nil)}).Optimize(t.Context(), p); err == nil {
		t.Fatal("want error; core falls back to the original")
	}
}
