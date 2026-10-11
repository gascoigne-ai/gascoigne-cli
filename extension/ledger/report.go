package ledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
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
	case component.ByAgent:
		tx, group, table = l.events(ctx), "events.agent", "events"
	case component.ByModel:
		tx, group, table = l.events(ctx), "events.model", "events"
	case component.ByCommand:
		tx, group, table = l.events(ctx), "events.command", "events"
	case component.ByOutcome:
		tx, group, table = l.events(ctx), "events.outcome", "events"
	case component.ByDay, component.ByWeek, component.ByMonth:
		tx, group, table = l.events(ctx), period(q.By), "events"
	case component.ByReason:
		return l.reasons(ctx, q)
	case component.ByFamily:
		tx, group, table = l.steps(ctx), "steps.family", "steps"
	case component.ByOptimizer:
		tx, group, table = l.steps(ctx), "steps.optimizer", "steps"
	case component.ByDialect:
		tx, group, table = l.steps(ctx), "steps.dialect", "steps"
	default:
		return nil, fmt.Errorf("stats: unknown grouping %q", q.By)
	}
	tx = filter(tx, "events", q)
	order := fmt.Sprintf("sum(%[1]s.raw_tokens) - sum(%[1]s.out_tokens) DESC, 1", table)
	if q.Chronological() {
		order = "1"
	}
	var rows []struct {
		Group                   string
		Family                  string
		Runs, Optimized, Failed int
		RawTokens, OutTokens    int
		FirstTS, LastTS         int64
	}
	err := tx.Select("CASE " + group + " WHEN '' THEN '-' ELSE " + group + " END AS \"group\", " + l.aggregates(q.By)).
		Group(group).
		Order(order).
		Scan(&rows).Error
	stats := make([]component.Stat, len(rows))
	for i, r := range rows {
		stats[i] = component.Stat{Group: r.Group, Family: r.Family, Runs: r.Runs, Optimized: r.Optimized, Failed: r.Failed,
			RawTokens: r.RawTokens, OutTokens: r.OutTokens, First: time.UnixMilli(r.FirstTS), Last: time.UnixMilli(r.LastTS)}
	}
	return stats, err
}

// period buckets events.ts (unix ms) in local time.
func period(by string) string {
	format := map[string]string{component.ByDay: "%Y-%m-%d", component.ByWeek: "%Y-W%W", component.ByMonth: "%Y-%m"}[by]
	return fmt.Sprintf("strftime('%s', events.ts / 1000, 'unixepoch', 'localtime')", format)
}

// filter applies the query's session, agent, model and time filters to an
// events table (or alias).
func filter(tx *gorm.DB, events string, q component.StatsQuery) *gorm.DB {
	for col, v := range map[string]string{"session": q.Session, "agent": q.Agent, "model": q.Model} {
		if v != "" {
			tx = tx.Where(events+"."+col+" = ?", v)
		}
	}
	if !q.Since.IsZero() {
		tx = tx.Where(events+".ts >= ?", q.Since.UnixMilli())
	}
	return tx
}

func (l *Ledger) events(ctx context.Context) *gorm.DB {
	return l.db.WithContext(ctx).Model(&event{})
}

func (l *Ledger) steps(ctx context.Context) *gorm.DB {
	return l.db.WithContext(ctx).Model(&step{}).Joins("JOIN events ON events.id = steps.event_id")
}

// aggregates: events count outcomes; every step is an optimization.
func (l *Ledger) aggregates(by string) string {
	if by == component.ByFamily || by == component.ByOptimizer || by == component.ByDialect {
		return `max(steps.family) AS family, count(*) AS runs, count(*) AS optimized, 0 AS failed,
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

// reasons groups runs that were not optimized by outcome, stopping component
// and command, each with its latest reason.
func (l *Ledger) reasons(ctx context.Context, q component.StatsQuery) ([]component.Stat, error) {
	var rows []struct {
		Outcome, Filter, Command   string
		Runs, RawTokens, OutTokens int
		FirstTS, LastTS            int64
		LastID                     uint
	}
	tx := filter(l.db.WithContext(ctx).Table("events e"), "e", q).
		Where("e.outcome <> ?", string(component.Optimized))
	err := tx.Select(`e.outcome, e.filter, e.command, count(*) AS runs,
			sum(e.raw_tokens) AS raw_tokens, sum(e.out_tokens) AS out_tokens,
			min(e.ts) AS first_ts, max(e.ts) AS last_ts, max(e.id) AS last_id`).
		Group("e.outcome, e.filter, e.command").
		Order("sum(e.raw_tokens) DESC, e.outcome, e.filter, e.command").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	stats := make([]component.Stat, len(rows))
	for i, r := range rows {
		// The latest event of the group carries the reason to show.
		var last event
		if err := l.db.WithContext(ctx).Select("reason").Take(&last, r.LastID).Error; err != nil {
			return nil, err
		}
		stats[i] = component.Stat{
			Group: cmp.Or(r.Filter, "-"), Command: r.Command, Outcome: r.Outcome, Reason: last.Reason,
			Runs: r.Runs, RawTokens: r.RawTokens, OutTokens: r.OutTokens,
			First: time.UnixMilli(r.FirstTS), Last: time.UnixMilli(r.LastTS),
		}
		if r.Outcome == string(component.Failed) {
			stats[i].Failed = r.Runs
		}
	}
	return stats, nil
}

// Entry is a recorded event with its row id, for feeds that ask what is new.
type Entry struct {
	ID uint
	component.Event
}

// Recent lists events matching q's filters with an id above after, newest
// first, at most limit.
func (l *Ledger) Recent(ctx context.Context, q component.StatsQuery, after uint, limit int) ([]Entry, error) {
	return l.Page(ctx, q, Page{After: after, Limit: limit})
}

// Page selects a slice of the run feed, newest first.
type Page struct {
	After  uint // only runs with a larger id (new ones, for a live feed)
	Before uint // only runs with a smaller id (older ones, loading on scroll); 0 = no bound
	// Outcomes keeps runs with these outcomes ("optimized", "unchanged",
	// "bypassed", "error"); empty = all.
	Outcomes []string
	Limit    int
}

// Page returns the runs matching q and p, newest first.
func (l *Ledger) Page(ctx context.Context, q component.StatsQuery, p Page) ([]Entry, error) {
	var rows []event
	tx := filter(l.db.WithContext(ctx).Preload("Steps"), "events", q).Where("events.id > ?", p.After)
	if p.Before > 0 {
		tx = tx.Where("events.id < ?", p.Before)
	}
	if len(p.Outcomes) > 0 {
		tx = tx.Where("events.outcome IN ?", p.Outcomes)
	}
	if err := tx.Order("events.id DESC").Limit(p.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Entry, len(rows))
	for i, r := range rows {
		out[i] = Entry{ID: r.ID, Event: r.component()}
	}
	return out, nil
}

func (r event) component() component.Event {
	steps := make([]component.Step, len(r.Steps))
	for i, s := range r.Steps {
		steps[i] = component.Step{Optimizer: s.Optimizer, Family: s.Family, Dialect: s.Dialect, RawTokens: s.RawTokens, OutTokens: s.OutTokens}
	}
	var optimizers []string
	if r.Optimizers != "" {
		optimizers = strings.Split(r.Optimizers, ",")
	}
	return component.Event{
		TS: time.UnixMilli(r.TS), Session: r.Session, Agent: r.Agent, Model: r.Model, Command: r.Command,
		Pipeline: r.Pipeline, Key: r.Key, Route: r.Route, Optimizers: optimizers,
		RawTokens: r.RawTokens, OutTokens: r.OutTokens, Duration: time.Duration(r.DurationMS) * time.Millisecond,
		RawID: r.RawID, Outcome: component.Outcome(r.Outcome), Filter: r.Filter, Reason: r.Reason, Steps: steps,
	}
}

// Reset deletes recorded events and their steps matching q's filters and
// reports how many events went. all also deletes stored originals and
// failure history, which carry no session or agent, so it takes no filters.
func (l *Ledger) Reset(ctx context.Context, q component.StatsQuery, all bool) (int64, error) {
	if all && (q.Session != "" || q.Agent != "" || q.Model != "" || !q.Since.IsZero()) {
		return 0, errors.New("reset: originals and failure history cannot be filtered; drop the filters or the full reset")
	}
	var n int64
	err := l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := filter(tx.Model(&event{}).Select("id"), "events", q)
		if err := tx.Where("event_id IN (?)", ids).Delete(&step{}).Error; err != nil {
			return err
		}
		res := filter(tx.Where("1 = 1"), "events", q).Delete(&event{})
		if res.Error != nil {
			return res.Error
		}
		n = res.RowsAffected
		if q.Session == "" { // usage has no session
			if err := usageFilter(tx.Where("1 = 1"), q).Delete(&usage{}).Error; err != nil {
				return err
			}
		}
		if !all {
			return nil
		}
		if err := tx.Where("1 = 1").Delete(&raw{}).Error; err != nil {
			return err
		}
		return tx.Where("1 = 1").Delete(&failure{}).Error
	})
	return n, err
}
