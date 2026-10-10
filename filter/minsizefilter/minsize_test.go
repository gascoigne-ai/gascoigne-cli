package minsizefilter

import (
	"testing"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

func TestFilter(t *testing.T) {
	f := &Filter{cfg: &Config{MinTokens: 2}}
	tests := []struct {
		text string
		want component.Decision
	}{
		{"abcd", component.Bypass},
		{"ab cd", component.Continue},
	}
	for _, tt := range tests {
		if got, _ := f.Filter(t.Context(), payload.New(payload.CmdOutput, tt.text)); got != tt.want {
			t.Errorf("Filter(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}
