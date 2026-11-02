package historyfilter

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type fakeLedger struct {
	component.Ledger
	component.Nop
	failures map[string]component.FailureInfo
	stats    []component.Stat
	queries  *[]component.StatsQuery
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

// Between "never failed" and "bypassed" a source that was fetched raw once
// gets the gentle route: less cut, so the next read may be enough.
func TestFilterGentle(t *testing.T) {
	l := fakeLedger{failures: map[string]component.FailureInfo{
		"cmd:ls":      fail(1, component.ReasonRawRequested, time.Minute),
		"cmd:git log": fail(2, component.ReasonRawRequested, time.Minute),
		"cmd:old":     fail(1, component.ReasonRawRequested, 48*time.Hour),
		"url:doc":     fail(2, component.ReasonRawRequested, time.Minute),
	}}
	tests := []struct {
		name        string
		gentleAfter int
		attr, value string
		want        component.Decision
		level       string
	}{
		{"never failed", 1, payload.AttrCmd, "none", component.Continue, ""},
		{"one raw request goes gentle", 1, payload.AttrCmd, "ls", component.Continue, LevelGentle},
		{"disabled", 0, payload.AttrCmd, "ls", component.Continue, ""},
		{"later threshold", 2, payload.AttrCmd, "ls", component.Continue, ""},
		{"at max still bypasses", 1, payload.AttrCmd, "git log", component.Bypass, ""},
		{"expired goes back to normal", 1, payload.AttrCmd, "old", component.Continue, ""},
		{"fetched page below its higher max", 1, payload.AttrURL, "doc", component.Continue, LevelGentle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MaxFailures: 2, FetchMaxFailures: 3, FailureTTL: 24 * time.Hour, GentleAfter: tt.gentleAfter}
			f := &Filter{cfg: cfg, log: slog.New(slog.DiscardHandler)}
			if err := f.Start(t.Context(), host{{Type: "ledger"}: l}); err != nil {
				t.Fatal(err)
			}
			p := payload.New(payload.CmdOutput, "x")
			p.Set(tt.attr, tt.value)
			if got, err := f.Filter(t.Context(), p); err != nil || got != tt.want {
				t.Fatalf("Filter = %v, %v; want %v", got, err, tt.want)
			}
			if got := p.Attrs[AttrLevel]; got != tt.level {
				t.Errorf("level = %q, want %q", got, tt.level)
			}
		})
	}
}

func TestValidateGentleAfter(t *testing.T) {
	for _, tt := range []struct {
		gentle int
		bad    bool
	}{{0, false}, {1, false}, {2, false}, {-1, true}} {
		c := Config{MaxFailures: 2, FetchMaxFailures: 3, GentleAfter: tt.gentle}
		if err := c.Validate(); (err != nil) != tt.bad {
			t.Errorf("gentle_after %d: Validate() = %v, want error %v", tt.gentle, err, tt.bad)
		}
	}
}

func (l fakeLedger) Stats(_ context.Context, q component.StatsQuery) ([]component.Stat, error) {
	*l.queries = append(*l.queries, q)
	return l.stats, nil
}

func famStat(family, outcome string, runs int) component.Stat {
	return component.Stat{Group: family, Family: family, Outcome: outcome, Runs: runs}
}

// A family whose results the agent keeps fetching raw goes gentle for every
// source, not only for the ones that failed one by one.
func TestFilterGentleFamily(t *testing.T) {
	stats := []component.Stat{
		famStat("log", "optimized", 30), famStat("log", "rerequested", 20), // 40%
		famStat("test", "optimized", 90), famStat("test", "rerequested", 10), // 10%
		famStat("vcs", "optimized", 2), famStat("vcs", "rerequested", 8), // 80% of 10 runs: too few
	}
	tests := []struct {
		name  string
		rate  float64
		block string
		level string
	}{
		{"family over the rate", 0.3, "log/app", LevelGentle},
		{"family under the rate", 0.3, "test/go", ""},
		{"too few runs", 0.3, "vcs/git-log", ""},
		{"unknown family", 0.3, "doc/markdown", ""},
		{"off", 0, "log/app", ""},
		{"any block of the payload counts", 0.3, "test/go+log/app", LevelGentle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var queries []component.StatsQuery
			l := fakeLedger{stats: stats, queries: &queries}
			cfg := &Config{MaxFailures: 2, FetchMaxFailures: 3, FamilyWindow: 7 * 24 * time.Hour, FamilyMinRuns: 20, FamilyRerequestRate: tt.rate}
			f := &Filter{cfg: cfg, log: slog.New(slog.DiscardHandler), now: time.Now}
			if err := f.Start(t.Context(), host{{Type: "ledger"}: l}); err != nil {
				t.Fatal(err)
			}
			p := payload.New(payload.CmdOutput, "x")
			p.Blocks = nil
			for typ := range strings.SplitSeq(tt.block, "+") {
				p.Blocks = append(p.Blocks, payload.Block{Text: "x", Type: typ})
			}
			if got, err := f.Filter(t.Context(), p); err != nil || got != component.Continue {
				t.Fatalf("Filter = %v, %v", got, err)
			}
			if got := p.Attrs[AttrLevel]; got != tt.level {
				t.Errorf("level = %q, want %q", got, tt.level)
			}
			if tt.rate > 0 && (len(queries) != 1 || queries[0].By != component.ByFamilyOutcome || queries[0].Since.IsZero()) {
				t.Errorf("queries = %+v, want one family_outcome query with a window", queries)
			}
		})
	}
}

// The family rates are read once a minute, not for every payload.
func TestFamilyRatesAreCached(t *testing.T) {
	var queries []component.StatsQuery
	l := fakeLedger{stats: []component.Stat{famStat("log", "optimized", 10), famStat("log", "rerequested", 30)}, queries: &queries}
	clock := time.Now()
	cfg := &Config{MaxFailures: 2, FetchMaxFailures: 3, FamilyWindow: time.Hour, FamilyMinRuns: 20, FamilyRerequestRate: 0.3}
	f := &Filter{cfg: cfg, log: slog.New(slog.DiscardHandler), now: func() time.Time { return clock }}
	if err := f.Start(t.Context(), host{{Type: "ledger"}: l}); err != nil {
		t.Fatal(err)
	}
	run := func() {
		p := payload.New(payload.CmdOutput, "x")
		p.Blocks[0].Type = "log/app"
		if _, err := f.Filter(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	run()
	run()
	clock = clock.Add(30 * time.Second)
	run()
	if len(queries) != 1 {
		t.Fatalf("%d queries within a minute, want 1", len(queries))
	}
	clock = clock.Add(time.Minute)
	run()
	if len(queries) != 2 {
		t.Fatalf("%d queries after a minute, want 2", len(queries))
	}
}
