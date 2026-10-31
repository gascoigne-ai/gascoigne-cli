package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContextNudge(t *testing.T) {
	assistant := func(sidechain bool, model string, in, read, write int) string {
		return fmt.Sprintf(`{"type":"assistant","isSidechain":%t,"message":{"id":"m","model":%q,"usage":{"input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens":9}}}`,
			sidechain, model, in, read, write)
	}
	user := `{"type":"user","message":{"role":"user","content":"go on"}}`
	big := assistant(false, "claude-opus-5-5", 5, 280000, 4000)
	small := assistant(false, "claude-opus-5-5", 5, 90000, 1000)
	const note = "gascoigne: context is 284k tokens: finish or /compact"
	tests := []struct {
		name  string
		lines []string
		env   string
		want  string
	}{
		{"above", []string{user, big, user}, "", note},
		{"below", []string{user, small, user}, "", ""},
		{"latest request wins", []string{big, assistant(false, "claude-opus-5-5", 5, 30000, 100)}, "", ""},
		{"subagent ignored", []string{small, assistant(true, "claude-opus-5-5", 5, 400000, 0)}, "", ""},
		{"synthetic ignored", []string{big, assistant(false, "<synthetic>", 0, 0, 0)}, "", note},
		{"cut first line", []string{`ache_read_input_tokens":1}}}`, big}, "", note},
		{"env threshold", []string{small}, "50000", "gascoigne: context is 91k tokens: finish or /compact"},
		{"env off", []string{big}, "off", ""},
		{"env zero", []string{big}, "0", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GASCOIGNE_CONTEXT_NUDGE", tt.env)
			path := filepath.Join(t.TempDir(), "t.jsonl")
			if err := os.WriteFile(path, []byte(strings.Join(tt.lines, "\n")+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := contextNudge(path); got != tt.want {
				t.Errorf("contextNudge = %q, want %q", got, tt.want)
			}
		})
	}
	if got := contextNudge(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Errorf("missing transcript: %q", got)
	}
}
