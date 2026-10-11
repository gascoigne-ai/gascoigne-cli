package embedjsonoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress/jsonx"
	"github.com/lukaszb/gascoigne/payload"
)

func orders(n int) string {
	var items []string
	for i := range n {
		items = append(items, fmt.Sprintf(`{"id":"ord_%d","status":"paid","currency":"EUR","total":%d}`, i, 10+i))
	}
	return `{"data":[` + strings.Join(items, ",") + `],"has_more":false}`
}

func run(t *testing.T, typ, text string) string {
	t.Helper()
	p := payload.New(payload.CmdOutput, text)
	p.Blocks[0].Type = typ
	cfg := &Config{Types: []string{"log", "test", "doc/text"}, JSON: *jsonx.Default()}
	if err := (&Optimizer{cfg: cfg}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p.Text()
}

func TestCrush(t *testing.T) {
	body := orders(30)
	tests := []struct {
		name, typ, in string
		crushed       bool
	}{
		{"labelled body in log", "log/app", "09:00 INFO got\n09:01 DEBUG body: " + body + "\n09:02 INFO done\n", true},
		{"body in test failure", "test/pytest", "FAILED test_sync\nE  assert " + body + " == {}\n", true},
		{"whole json block", "doc/text", body + "\n", false},
		{"no records", "log/app", "09:00 DEBUG cfg: {\"retries\":3,\"timeout\":\"5s\"}\n", false},
		{"scalar list", "log/app", "09:00 DEBUG ids: [1,2,3,4,5,6,7,8,9,10]\n", false},
		{"not a wanted type", "vcs/git-diff", "+ body: " + body + "\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := run(t, tt.typ, tt.in)
			if got := out != tt.in; got != tt.crushed {
				t.Fatalf("crushed = %v, want %v\n%s", got, tt.crushed, out)
			}
		})
	}
}

func TestCrushKeepsTextAround(t *testing.T) {
	in := "09:00 INFO start\n09:01 DEBUG body: " + orders(30) + " (312ms)\n09:02 INFO done\n"
	out := run(t, "log/app", in)
	lines := strings.Split(out, "\n")
	if lines[0] != "// "+jsonx.Legend || lines[1] != "09:00 INFO start" || lines[3] != "09:02 INFO done" {
		t.Fatalf("lines around changed:\n%s", out)
	}
	if !strings.HasPrefix(lines[2], `09:01 DEBUG body: {"data":{`) || !strings.HasSuffix(lines[2], "} (312ms)") {
		t.Fatalf("body line:\n%s", lines[2])
	}
	if again := run(t, "log/app", out); again != out {
		t.Fatalf("not idempotent:\n%s", again)
	}
}
