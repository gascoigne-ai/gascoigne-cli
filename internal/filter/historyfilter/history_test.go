package historyfilter

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/payload"
)

type fakeLedger struct {
	component.Ledger
	component.Nop
	failures map[string]int
}

func (l fakeLedger) Failures(_ context.Context, key string) (int, error) { return l.failures[key], nil }

type host map[component.ID]component.Component

func (h host) Extension(id component.ID) (component.Component, bool) {
	c, ok := h[id]
	return c, ok
}

func TestFilter(t *testing.T) {
	l := fakeLedger{failures: map[string]int{"cmd:ls": 1, "cmd:git log": 2}}
	f := &Filter{cfg: &Config{Ledger: "ledger", MaxFailures: 2}, log: slog.New(slog.DiscardHandler)}
	if err := f.Start(t.Context(), host{{Type: "ledger"}: l}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		cmd  string
		want component.Decision
	}{
		{"", component.Continue},
		{"ls", component.Continue},
		{"git log", component.Bypass},
	}
	for _, tt := range tests {
		p := payload.New(payload.CmdOutput, "x")
		if tt.cmd != "" {
			p.Set(payload.AttrCmd, tt.cmd)
		}
		if got, err := f.Filter(t.Context(), p); err != nil || got != tt.want {
			t.Errorf("Filter(%q) = %v, %v; want %v", tt.cmd, got, err, tt.want)
		}
	}
}

func TestStartMissingLedger(t *testing.T) {
	f := &Filter{cfg: &Config{Ledger: "ledger", MaxFailures: 1}}
	if err := f.Start(t.Context(), host{}); err == nil {
		t.Fatal("want error")
	}
}
