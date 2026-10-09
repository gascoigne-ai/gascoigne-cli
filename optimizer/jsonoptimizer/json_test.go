package jsonoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func cfg() *Config {
	return &Config{DropEmpty: true, MaxItems: 8, KeepEnds: 2, TabulateMin: 3, MaxString: 40, ShortenHashes: true}
}

func run(t *testing.T, typ, in string) string {
	t.Helper()
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: typ}}}
	if err := (&Optimizer{cfg: cfg()}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(p.Blocks[0].Text, "// "+legend+"\n")
}

func TestDropEmptyAndStrings(t *testing.T) {
	in := `{"a": null, "b": "", "c": [], "d": {}, "e": 0, "f": false, "long": "` + strings.Repeat("x", 100) +
		`", "sha": "sha512-Aup7aUOfp1234567890abcdefghijklmnopqrstuvwxyzABCDEFGHIJ"}`
	got := run(t, detect.JSON, in)
	want := `{"e":0,"f":false,"long":"xxxxxxxxxxxxxxxxxxxx … xxxxxxxxxxxxxxxxxxxx","sha":"sha512-Aup7aUOfp…"}` + "\n"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestTableWithSameAndAffix(t *testing.T) {
	in := `[{"name":"a","owner":"go","url":"https://x.io/repos/a/hooks"},
	         {"name":"b","owner":"go","url":"https://x.io/repos/b/hooks"},
	         {"name":"c","owner":"go","url":"https://x.io/repos/c/hooks"}]`
	got := run(t, detect.JSON, in)
	want := `{"_affix":{"url":["https://x.io/repos/","/hooks"]},"_cols":["name","url"],"_rows":[["a","a"],["b","b"],["c","c"]],"_same":{"owner":"go"}}` + "\n"
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestSampleKeepsSignal(t *testing.T) {
	var items []string
	for i := range 40 {
		level, dur := "info", 10+i%5
		switch i {
		case 17:
			level = "error"
		case 23:
			dur = 5000
		}
		items = append(items, fmt.Sprintf(`{"i":%d,"level":%q,"dur":%d}`, i, level, dur))
	}
	got := run(t, detect.JSON, "["+strings.Join(items, ",")+"]")
	for _, want := range []string{`[10,0,"info"]`, `[11,1,"info"]`, `"error"`, `5000`, `[14,39,"info"]`, "items omitted"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, `[10,5,"info"]`) {
		t.Errorf("kept a boring middle item: %s", got)
	}
}

func TestStreamAndNDJSONBecomeTables(t *testing.T) {
	for _, typ := range []string{detect.JSON, detect.NDJSON} {
		got := run(t, typ, "{\"a\":1,\"b\":2}\n{\"a\":3,\"b\":4}\n{\"a\":5,\"b\":6}\n")
		if want := `{"_cols":["a","b"],"_rows":[[1,2],[3,4],[5,6]]}` + "\n"; got != want {
			t.Errorf("%s: got %s", typ, got)
		}
	}
}

func TestDeterministic(t *testing.T) {
	in := `{"z":{"k1":{"v":1,"w":2},"k2":{"v":3,"w":4},"k3":{"v":5,"w":6}},"a":[3,2,1]}`
	if a, b := run(t, detect.JSON, in), run(t, detect.JSON, in); a != b {
		t.Fatalf("nondeterministic:\n%s\n%s", a, b)
	}
}

func TestInvalidJSONErrors(t *testing.T) {
	p := &payload.Payload{Blocks: []payload.Block{{Text: `{"a":`, Type: detect.JSON}}}
	if err := (&Optimizer{cfg: cfg()}).Optimize(t.Context(), p); err == nil {
		t.Fatal("want error; core falls back to the original")
	}
}
