package grepoptimizer

import (
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
		b.WriteString("a.go:" + string(rune('0'+i%10)) + "1:" + strings.Repeat("x", 300) + "\n")
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
