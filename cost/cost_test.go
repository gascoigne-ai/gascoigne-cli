package cost

import (
	"math"
	"testing"
)

func TestLookup(t *testing.T) {
	tests := []struct{ in, want string }{
		{"claude-opus-5-5", "claude-opus-5-5"},
		{"claude-opus-5-5[1m]", "claude-opus-5-5"},
		{"Claude-Sonnet-5-5", "claude-sonnet-5-5"},
		{"claude-sonnet-5", "claude-sonnet-5"},
		{"gpt-5.4-mini-2026-03-01", "gpt-5.4-mini"},
		{"gpt-5.3-codex", "gpt-5.3-codex"},
		{"gemini-3.1-pro-preview", "gemini-3.1-pro"},
		// Variants priced apart must not fall back to their base model's prefix.
		{"gpt-5.5-pro", "gpt-5.5-pro"},
		{"gpt-5.6-terra", "gpt-5.6-terra"},
		{"gemini-2.5-flash-lite", "gemini-2.5-flash-lite"},
		{"claude-haiku-5-5", "claude-haiku-5-5"},
		{"gpt-6.1-sol", "gpt-6.1-sol"},
		{"llama-4", ""},
	}
	for _, tt := range tests {
		m, ok := Lookup(tt.in)
		if m.ID != tt.want || ok != (tt.want != "") {
			t.Errorf("Lookup(%q) = %q, %v; want %q", tt.in, m.ID, ok, tt.want)
		}
	}
}

func TestEveryModelHasATokenizerAndAgentDefaultsExist(t *testing.T) {
	for _, m := range Models {
		if _, ok := Tokenizers[m.Tokenizer]; !ok {
			t.Errorf("%s: unknown tokenizer %q", m.ID, m.Tokenizer)
		}
		if m.CacheRead > m.Input || m.CacheWrite < m.Input {
			t.Errorf("%s: cache prices out of order: %+v", m.ID, m)
		}
	}
	for agent, id := range AgentModels {
		if _, ok := Lookup(id); !ok {
			t.Errorf("agent %s defaults to unknown model %q", agent, id)
		}
	}
}

func TestEstimate(t *testing.T) {
	m, _ := Lookup("claude-opus-5-5") // $4 input, $5 write, $0.20 read
	s := Estimate(m, 1_000_000, Context{Rereads: 10, Miss: 0})
	if s.ModelTokens != 1_580_000 || !near(s.FirstRead, 1.58*5) || !near(s.InContext, 1.58*(5+10*0.20)) {
		t.Fatalf("got %+v", s)
	}
	g, _ := Lookup("gpt-5.5") // no write premium: first read at input price
	if s := Estimate(g, 1_000_000, Context{Rereads: 1, Miss: 1}); !near(s.FirstRead, 5) || !near(s.InContext, 10) {
		t.Fatalf("missed cache should pay input again: %+v", s)
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
