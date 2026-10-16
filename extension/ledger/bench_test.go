package ledger

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

// benchEvents is the size of the seeded history: a few months of heavy use.
const benchEvents = 20_000

// seeded fills a ledger with n events spread over the last 800 days, a third
// of them with optimizer steps, straight through gorm in batches so setup
// stays fast.
func seeded(tb testing.TB, n int) *Ledger {
	tb.Helper()
	l := New(&Config{Path: filepath.Join(tb.TempDir(), "l.db")})
	if err := l.Start(tb.Context(), nil); err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { l.Shutdown(tb.Context()) })
	rng := rand.New(rand.NewPCG(42, 7))
	pick := func(prefix string, k int) string { return fmt.Sprint(prefix, rng.IntN(k)) }
	start := l.now().Add(-800 * 24 * time.Hour)
	events := make([]event, n)
	for i := range events {
		raw := 50 + rng.IntN(20_000)
		e := event{
			TS: start.Add(time.Duration(i) * (800 * 24 * time.Hour / time.Duration(n))).UnixMilli(), Session: pick("s", 400),
			Agent: pick("a", 4), Model: pick("m", 6), Command: pick("cmd", 60), Pipeline: "cli", Key: pick("k", 3000),
			Route: pick("r", 8), RawTokens: raw, OutTokens: raw, Outcome: string(component.Unchanged),
		}
		if rng.IntN(3) == 0 {
			e.Outcome, e.OutTokens = string(component.Optimized), 1+rng.IntN(raw)
			for range 1 + rng.IntN(3) {
				e.Steps = append(e.Steps, step{Optimizer: pick("o", 30), Family: pick("f", 10), Dialect: pick("d", 100), RawTokens: raw, OutTokens: e.OutTokens})
			}
		}
		events[i] = e
	}
	if err := l.db.WithContext(tb.Context()).CreateInBatches(events, 500).Error; err != nil {
		tb.Fatal(err)
	}
	return l
}

func totals(tb testing.TB, l *Ledger, by string) (runs, raw, out int) {
	tb.Helper()
	stats, err := l.Stats(tb.Context(), component.StatsQuery{By: by})
	if err != nil {
		tb.Fatal(err)
	}
	for _, s := range stats {
		runs, raw, out = runs+s.Runs, raw+s.RawTokens, out+s.OutTokens
	}
	return
}

// A year of history rolled up at scale keeps every total.
func TestRollupAtScale(t *testing.T) {
	if testing.Short() {
		t.Skip("seeds 20k events")
	}
	l := seeded(t, benchEvents)
	var before [3][3]int
	groupings := []string{component.ByDay, component.ByMonth, component.ByCommand, component.ByOptimizer}
	for i, by := range groupings[:3] {
		before[i][0], before[i][1], before[i][2] = totals(t, l, by)
	}
	l.cfg.EventsTTL = 365 * 24 * time.Hour
	if err := l.pruneEvents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var left int64
	l.db.Model(&event{}).Count(&left)
	if left == 0 || left >= benchEvents {
		t.Fatalf("%d events left of %d, want only the last year", left, benchEvents)
	}
	for i, by := range groupings[:3] {
		if r, a, o := totals(t, l, by); [3]int{r, a, o} != before[i] {
			t.Errorf("by %s: totals after rollup %v, before %v", by, [3]int{r, a, o}, before[i])
		}
	}
}

func BenchmarkRollUp(b *testing.B) {
	for b.Loop() {
		b.StopTimer()
		l := seeded(b, benchEvents)
		l.cfg.EventsTTL = 365 * 24 * time.Hour
		b.StartTimer()
		if err := l.pruneEvents(b.Context()); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkStats compares the groupings gain prints with all runs stored and
// with the older half rolled up.
func BenchmarkStats(b *testing.B) {
	for _, rolled := range []bool{false, true} {
		l := seeded(b, benchEvents)
		if rolled {
			l.cfg.EventsTTL = 400 * 24 * time.Hour
			if err := l.pruneEvents(b.Context()); err != nil {
				b.Fatal(err)
			}
		}
		for _, by := range []string{component.ByDay, component.ByMonth, component.ByCommand, component.ByAgent, component.ByOptimizer, component.ByDialect, component.BySession} {
			b.Run(fmt.Sprintf("rolled=%v/%s", rolled, by), func(b *testing.B) {
				for b.Loop() {
					if _, err := l.Stats(b.Context(), component.StatsQuery{By: by}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
