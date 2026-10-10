package truncateoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/tokens"
)

func numbered(n int, mark map[int]string) string {
	var b strings.Builder
	for i := range n {
		if m, ok := mark[i]; ok {
			b.WriteString(m + "\n")
			continue
		}
		fmt.Fprintf(&b, "line %04d of some ordinary progress output here\n", i)
	}
	return b.String()
}

func run(t *testing.T, cfg *Config, typ, text string) string {
	t.Helper()
	p := payload.New(payload.CmdOutput, text)
	p.Blocks[0].Type = typ
	if err := (&Optimizer{cfg: cfg}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p.Text()
}

func TestCut(t *testing.T) {
	cfg := &Config{MaxTokens: 1000, HeadTokens: 200, TailTokens: 200, MaxErrors: 2, Skip: []string{"fs/code", "data"}}
	big := numbered(1000, map[int]string{400: "ERROR: disk full", 500: "panic: boom", 600: "error: third"})
	tests := []struct {
		name, typ, text string
		keep, drop      []string
	}{
		{"small stays", "log/app", numbered(10, nil), []string{"line 0009"}, nil},
		{"head, errors, tail", "log/app", big,
			[]string{"line 0000", "line 0999", "ERROR: disk full", "panic: boom", "lines omitted"},
			[]string{"line 0501", "error: third"}},
		{"skipped type", "fs/code", big, []string{"line 0501"}, nil},
		{"skipped family", "data/json", big, []string{"line 0501"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, cfg, tt.typ, tt.text)
			for _, k := range tt.keep {
				if !strings.Contains(got, k) {
					t.Errorf("lost %q", k)
				}
			}
			for _, d := range tt.drop {
				if strings.Contains(got, d) {
					t.Errorf("kept %q", d)
				}
			}
		})
	}
	if got := run(t, cfg, "log/app", big); tokens.Estimate(got) > cfg.MaxTokens || run(t, cfg, "log/app", got) != got {
		t.Fatalf("over budget (%d tokens) or not idempotent", tokens.Estimate(got))
	}
}

func TestLongLines(t *testing.T) {
	cfg := defaults()
	got := run(t, cfg, "doc/text", strings.Repeat("ab+", 30000)+"\n")
	if tokens.Estimate(got) > 2000 {
		t.Fatalf("one long line kept at %d tokens", tokens.Estimate(got))
	}
}

func TestValidate(t *testing.T) {
	if err := (&Config{MaxTokens: 100, HeadTokens: 60, TailTokens: 40}).Validate(); err == nil {
		t.Fatal("head + tail = max accepted")
	}
	if err := defaults().Validate(); err != nil {
		t.Fatal(err)
	}
}
