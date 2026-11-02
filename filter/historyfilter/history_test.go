package historyfilter

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type fakeLedger struct {
	component.Ledger
	component.Nop
	failures map[string]component.FailureInfo
}

func (l fakeLedger) Failures(_ context.Context, key string) (component.FailureInfo, error) {
	return l.failures[key], nil
}

type host map[component.ID]component.Component

func (h host) Extension(id component.ID) (component.Component, bool) {
	c, ok := h[id]
	return c, ok
}

func fail(n int, reason string, ago time.Duration) component.FailureInfo {
	return component.FailureInfo{Count: n, Reason: reason, Last: time.Now().Add(-ago)}
}

// One failure bypassed its source forever, and a full read of a fetched doc
// page counted like a bad command; both cost raw tokens on later runs.
func TestFilter(t *testing.T) {
	l := fakeLedger{failures: map[string]component.FailureInfo{
		"cmd:ls":         fail(1, component.ReasonError, time.Minute),
		"cmd:git log":    fail(2, component.ReasonError, time.Minute),
		"cmd:old":        fail(5, component.ReasonError, 48*time.Hour),
		"url:doc":        fail(2, component.ReasonRawRequested, time.Minute),
		"url:doc3":       fail(3, component.ReasonRawRequested, time.Minute),
		"url:err":        fail(2, component.ReasonError, time.Minute),
		"url:doc3-stale": fail(3, component.ReasonRawRequested, 48*time.Hour),
	}}
	cfg := &Config{MaxFailures: 2, FetchMaxFailures: 3, FailureTTL: 24 * time.Hour}
	f := &Filter{cfg: cfg, log: slog.New(slog.DiscardHandler)}
	if err := f.Start(t.Context(), host{{Type: "ledger"}: l}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		attr  string
		value string
		want  component.Decision
	}{
		{"no key", "", "", component.Continue},
		{"below max", payload.AttrCmd, "ls", component.Continue},
		{"at max", payload.AttrCmd, "git log", component.Bypass},
		{"expired by ttl", payload.AttrCmd, "old", component.Continue},
		{"full read of a page weighs less", payload.AttrURL, "doc", component.Continue},
		{"page at fetch max", payload.AttrURL, "doc3", component.Bypass},
		{"page error counts as a command", payload.AttrURL, "err", component.Bypass},
		{"page expired by ttl", payload.AttrURL, "doc3-stale", component.Continue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := payload.New(payload.CmdOutput, "x")
			if tt.attr != "" {
				p.Set(tt.attr, tt.value)
			}
			if got, err := f.Filter(t.Context(), p); err != nil || got != tt.want {
				t.Errorf("Filter = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}

func TestStartMissingLedger(t *testing.T) {
	f := &Filter{cfg: &Config{MaxFailures: 1}}
	if err := f.Start(t.Context(), host{}); err == nil {
		t.Fatal("want error")
	}
}
