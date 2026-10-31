package main

import (
	"strings"
	"testing"
)

// A whole original over ~2k tokens starts with a size header so the agent can pick
// -grep/-lines/-query next time; small and partial reads stay byte-exact.
func TestRawHeader(t *testing.T) {
	big := strings.Repeat("some output line with words\n", 600)
	tests := []struct {
		name    string
		text    string
		partial bool
		want    bool
	}{
		{"small", "ok\n", false, false},
		{"big", big, false, true},
		{"big partial", big, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := rawHeader(tt.text, tt.partial)
			if got := h != ""; got != tt.want {
				t.Fatalf("header %q, want header %v", h, tt.want)
			}
			if tt.want && (!strings.HasSuffix(h, "\n") || !strings.Contains(h, "tokens; -grep")) {
				t.Errorf("header %q", h)
			}
		})
	}
}
