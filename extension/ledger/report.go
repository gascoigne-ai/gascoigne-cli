package ledger

import (
	"cmp"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/lukaszb/gascoigne/component"
)

// Stats aggregates events by route or session, or optimizer steps by family
// or optimizer, optionally within one session; biggest savings first.
func (l *Ledger) Stats(ctx context.Context, q component.StatsQuery) ([]component.Stat, error) {
	var (
		tx           *gorm.DB
		group, table string
	)
	switch cmp.Or(q.By, component.ByRoute) {
	case component.ByRoute:
		tx, group, table = l.events(ctx), "events.route", "events"
	case component.BySession:
		tx, group, table = l.events(ctx), "events.session", "events"
	case component.ByFamily:
		tx, group, table = l.steps(ctx), "steps.family", "steps"
	case component.ByOptimizer:
		tx, group, table = l.steps(ctx), "steps.optimizer", "steps"
	default:
		return nil, fmt.Errorf("stats: unknown grouping %q", q.By)
	}
	if q.Session != "" {
		tx = tx.Where("events.session = ?", q.Session)
	}
	var rows []struct {
		Group                   string
		Runs, Optimized, Failed int
		RawTokens, OutTokens    int
		FirstTS, LastTS         int64
	}
	err := tx.Select("CASE " + group + " WHEN '' THEN '-' ELSE " + group + " END AS \"group\", " + l.aggregates(q.By)).
		Group(group).
		Order(fmt.Sprintf("sum(%[1]s.raw_tokens) - sum(%[1]s.out_tokens) DESC, 1", table)).
		Scan(&rows).Error
	stats := make([]component.Stat, len(rows))
	for i, r := range rows {
		stats[i] = component.Stat{Group: r.Group, Runs: r.Runs, Optimized: r.Optimized, Failed: r.Failed,
			RawTokens: r.RawTokens, OutTokens: r.OutTokens, First: time.UnixMilli(r.FirstTS), Last: time.UnixMilli(r.LastTS)}
	}
	return stats, err
}

func (l *Ledger) events(ctx context.Context) *gorm.DB {
	return l.db.WithContext(ctx).Model(&event{})
}

func (l *Ledger) steps(ctx context.Context) *gorm.DB {
	return l.db.WithContext(ctx).Model(&step{}).Joins("JOIN events ON events.id = steps.event_id")
}

// aggregates: events count outcomes; every step is an optimization.
func (l *Ledger) aggregates(by string) string {
	if by == component.ByFamily || by == component.ByOptimizer {
		return `count(*) AS runs, count(*) AS optimized, 0 AS failed,
			sum(steps.raw_tokens) AS raw_tokens, sum(steps.out_tokens) AS out_tokens,
			min(events.ts) AS first_ts, max(events.ts) AS last_ts`
	}
	return `count(*) AS runs, sum(outcome = 'optimized') AS optimized, sum(outcome = 'error') AS failed,
		sum(raw_tokens) AS raw_tokens, sum(out_tokens) AS out_tokens,
		min(ts) AS first_ts, max(ts) AS last_ts`
}

type Failure struct {
	Key    string
	Count  int
	Last   time.Time
	Reason string
}

func (l *Ledger) FailureList(ctx context.Context) ([]Failure, error) {
	var rows []failure
	if err := l.db.WithContext(ctx).Order("last_ts DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	fs := make([]Failure, len(rows))
	for i, r := range rows {
		fs[i] = Failure{r.Key, r.Count, time.UnixMilli(r.LastTS), r.Reason}
	}
	return fs, nil
}
