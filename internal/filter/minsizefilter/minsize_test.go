package minsizefilter

import (
	"testing"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/payload"
)

func TestFilter(t *testing.T) {
	f := &Filter{cfg: &Config{MinTokens: 2}}
	tests := []struct {
		text string
		want component.Decision
	}{
		{"abcd", component.Bypass},
		{"abcde", component.Continue},
	}
	for _, tt := range tests {
		if got, _ := f.Filter(t.Context(), payload.New(payload.CmdOutput, tt.text)); got != tt.want {
			t.Errorf("Filter(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
