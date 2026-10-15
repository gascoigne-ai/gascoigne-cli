package denseoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func run(t *testing.T, typ, text string, attrs ...string) string {
	t.Helper()
	p := payload.New(payload.CmdOutput, text)
	p.Blocks[0].Type = typ
	for i := 0; i+1 < len(attrs); i += 2 {
		p.Attrs[attrs[i]] = attrs[i+1]
	}
	if err := (&Optimizer{cfg: defaults()}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p.Text()
}

func TestElide(t *testing.T) {
	blob := strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo0", 100) // 3600 bytes of base64
	minified := strings.Repeat("function(a,b){return a.map(c=>c+b)};var x=", 80)
	json := `{"items":[` + strings.Repeat(`{"id":1,"name":"widget"},`, 150) + `{"id":2}]}`
	tsv := strings.Repeat("a\tbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\t", 120)
	prose := strings.Repeat("the quick brown fox jumps over the lazy dog ", 80)

	tests := []struct {
		name, typ, in string
		attrs         []string
		elided        bool
	}{
		{"base64 blob", "log/app", "start\n" + blob + "\nend\n", nil, true},
		{"log read from disk", "log/app", blob + "\n", []string{"cmd", "cat /var/log/app.log"}, true},
		{"minified bundle", "doc/text", minified + "\n", nil, true},
		{"fetched code", "fs/code", minified + "\n", []string{"cmd", "curl -s https://cdn.example.com/app.min.js"}, true},
		{"code read with cat", "fs/code", minified + "\n", []string{"cmd", "cat dist/app.min.js"}, false},
		{"code with file attr", "fs/code", minified + "\n", []string{"file", "dist/app.min.js"}, false},
		{"json line", "log/app", json + "\n", nil, false},
		{"labelled json", "log/app", "response: " + json + "\n", nil, false},
		{"tsv rows", "table/aligned", tsv + "\n", nil, false},
		{"prose", "doc/text", prose + "\n", nil, false},
		{"short dense total", "log/app", blob[:1500] + "\n", nil, false},
		{"data family", "data/yaml", blob + "\n", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := run(t, tt.typ, tt.in, tt.attrs...)
			if got := out != tt.in; got != tt.elided {
				t.Fatalf("elided = %v, want %v\n%s", got, tt.elided, out)
			}
		})
	}
}

func TestCutKeepsEnds(t *testing.T) {
	line := "data:image/png;base64," + strings.Repeat("iVBORw0KGgoAAAANSUhEUgAA", 120) + "END"
	out := run(t, "web/http", "before\n"+line+"\nafter\n")
	want := line[:160] + " …[" + "2665 bytes dense]… " + line[len(line)-80:]
	if out != "before\n"+want+"\nafter\n" {
		t.Fatalf("got\n%s", out)
	}
	if again := run(t, "web/http", out); again != out {
		t.Fatalf("not idempotent:\n%s", again)
	}
}

func TestCutRuneBoundary(t *testing.T) {
	line := strings.Repeat("é", 1500) // 2 bytes each, odd offsets fall mid-rune
	out := (&Optimizer{cfg: &Config{MinLine: 300, MinTotal: 0, Head: 161, Tail: 81}}).cut(line)
	if !strings.HasPrefix(out, strings.Repeat("é", 80)+" …[") || !strings.HasSuffix(out, "]… "+strings.Repeat("é", 41)) {
		t.Fatalf("cut mid-rune: %q", out[:200])
	}
}

func TestZeroTail(t *testing.T) {
	cfg := defaults()
	cfg.Tail = 0
	p := payload.New(payload.CmdOutput, "start\n"+strings.Repeat("QUJDREVGR0hJSktMTU5PUFFSU1RVVldYWVo0", 100)+"\nend\n")
	p.Blocks[0].Type = "log/app"
	if err := (&Optimizer{cfg: cfg}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
}
