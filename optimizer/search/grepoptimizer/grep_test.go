package grepoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func run(cfg *Config, in string) string {
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.Grep}}}
	(&Optimizer{cfg: cfg}).Optimize(nil, p)
	return p.Blocks[0].Text
}

func TestGroup(t *testing.T) {
	in := "./a.go:3:func A() {\n./a.go:9:\treturn a\nb/c.go:12:var C = 1\nb/c.go-13-// context\n--\nBinary file x matches\n"
	want := "Binary file x matches\na.go\n  3: func A() {\n  9: return a\nb/c.go\n  12: var C = 1\n  13- // context\n"
	if got := run(&Config{MaxPerFile: 20, MaxFiles: 100, MaxLineLen: 160}, in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCaps(t *testing.T) {
	var b strings.Builder
	for i := range 30 {
		fmt.Fprintf(&b, "a.go:%d:%s%d\n", i+1, strings.Repeat("x", 300), i)
	}
	b.WriteString("b.go:1:y\nc.go:1:z\n")
	got := run(&Config{MaxPerFile: 5, MaxFiles: 2, MaxLineLen: 40}, b.String())
	for _, want := range []string{"… 25 more matches omitted", "… 1 file omitted", "x … x"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "c.go") {
		t.Errorf("file cap ignored:\n%s", got)
	}
}

func TestPlain(t *testing.T) {
	cfg := &Config{MaxPerFile: 20, MaxFiles: 100, MaxLineLen: 160}
	tests := []struct{ name, in, want string }{
		{"rg without line numbers",
			"a/x.go:\tif err != nil {\nb.go:return err\na/x.go:\t\tif err != nil {\n",
			"a/x.go\n  2× if err != nil {\nb.go\n  return err\n"},
		{"single file grep stays", "key.name: 1\nother.key: 2\n", "key.name: 1\nother.key: 2\n"},
		{"prose stays", "note: see a.go\nnote: twice\n", "note: see a.go\nnote: twice\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(cfg, tt.in); got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestSameLinesMerged(t *testing.T) {
	in := "a.go:3:\tif err != nil {\na.go:4:\t\treturn err\na.go:9:\tif err != nil {\na.go:10-\t\treturn err\n"
	want := "a.go\n  3,9: if err != nil {\n  4: return err\n  10- return err\n"
	if got := run(&Config{MaxPerFile: 20, MaxFiles: 100, MaxLineLen: 160}, in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}
