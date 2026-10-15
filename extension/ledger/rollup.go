package ledger

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const (
	dayLocal = "strftime('%Y-%m-%d', ts / 1000, 'unixepoch', 'localtime')"
	upsert   = ` ON CONFLICT DO UPDATE SET runs = runs + excluded.runs, raw_tokens = raw_tokens + excluded.raw_tokens,
		out_tokens = out_tokens + excluded.out_tokens, first_ts = min(first_ts, excluded.first_ts),
		last_ts = max(last_ts, excluded.last_ts)`
)

// rollUp adds the runs and steps before cutoff (unix ms) to the daily totals.
func rollUp(tx *gorm.DB, cutoff int64) error {
	err := tx.Exec(`INSERT INTO daily_stats (day, agent, model, command, route, outcome, runs, raw_tokens, out_tokens, first_ts, last_ts)
		SELECT `+dayLocal+` AS d, agent, model, command, route, outcome, count(*), sum(raw_tokens), sum(`+eventOut+`), min(ts), max(ts)
		FROM events WHERE ts < ? GROUP BY d, agent, model, command, route, outcome`+upsert, cutoff).Error
	if err != nil {
		return fmt.Errorf("roll up runs: %w", err)
	}
	err = tx.Exec(`INSERT INTO daily_step_stats (day, agent, model, optimizer, family, dialect, runs, raw_tokens, out_tokens, first_ts, last_ts)
		SELECT `+dayLocal+` AS d, events.agent, events.model, steps.optimizer, steps.family, steps.dialect,
			count(*), sum(steps.raw_tokens), sum(steps.out_tokens), min(events.ts), max(events.ts)
		FROM steps JOIN events ON events.id = steps.event_id WHERE events.ts < ?
		GROUP BY d, events.agent, events.model, steps.optimizer, steps.family, steps.dialect`+upsert, cutoff).Error
	if err != nil {
		return fmt.Errorf("roll up steps: %w", err)
	}
	return nil
}

// pruneRollup drops daily totals older than RollupTTL; 0 keeps them forever.
func (l *Ledger) pruneRollup(ctx context.Context) error {
	if l.cfg.RollupTTL == 0 {
		return nil
	}
	cutoff := l.now().Add(-l.cfg.RollupTTL).Format(dayLayout)
	db := l.db.WithContext(ctx)
	if err := db.Where("day < ?", cutoff).Delete(&dailyStat{}).Error; err != nil {
		return err
	}
	return db.Where("day < ?", cutoff).Delete(&dailyStepStat{}).Error
}

// Stats is liveStats plus the daily totals of runs pruned at events_ttl, for
// every grouping they can answer (not session or reason). A rolled-up day
// counts whole when the query's Since falls inside it.
func (l *Ledger) Stats(ctx context.Context, q component.StatsQuery) ([]component.Stat, error) {
	live, err := l.liveStats(ctx, q)
	if err != nil || q.Session != "" || q.By == component.BySession || q.By == component.ByReason {
		return live, err
	}
	old, err := l.rolledStats(ctx, q)
	if err != nil || len(old) == 0 {
		return live, err
	}
	return mergeStats(q, live, old), nil
}

// rolledStats is liveStats over the daily totals.
func (l *Ledger) rolledStats(ctx context.Context, q component.StatsQuery) ([]component.Stat, error) {
	table, group, sel := "daily_stats", "", `sum(runs) AS runs, sum(CASE outcome WHEN 'optimized' THEN runs ELSE 0 END) AS optimized,
		sum(CASE outcome WHEN 'error' THEN runs ELSE 0 END) AS failed, sum(raw_tokens) AS raw_tokens, sum(out_tokens) AS out_tokens,
		min(first_ts) AS first_ts, max(last_ts) AS last_ts`
	switch by := cmp.Or(q.By, component.ByRoute); by {
	case component.ByRoute, component.ByAgent, component.ByModel, component.ByCommand, component.ByOutcome:
		group = by
	case component.ByDay:
		group = "day"
	case component.ByWeek:
		group = "strftime('%Y-W%W', day)"
	case component.ByMonth:
		group = "strftime('%Y-%m', day)"
	case component.ByFamily, component.ByOptimizer, component.ByDialect:
		table, group = "daily_step_stats", by
		sel = `max(family) AS family, sum(runs) AS runs, sum(runs) AS optimized, 0 AS failed, sum(raw_tokens) AS raw_tokens,
			sum(out_tokens) AS out_tokens, min(first_ts) AS first_ts, max(last_ts) AS last_ts`
	default:
		return nil, fmt.Errorf("stats: unknown grouping %q", q.By)
	}
	tx := l.db.WithContext(ctx).Table(table)
	for col, v := range map[string]string{"agent": q.Agent, "model": q.Model} {
		if v != "" {
			tx = tx.Where(col+" = ?", v)
		}
	}
	if !q.Since.IsZero() {
		tx = tx.Where("day >= ?", q.Since.Local().Format(dayLayout))
	}
	var rows []struct {
		Group                   string
		Family                  string
		Runs, Optimized, Failed int
		RawTokens, OutTokens    int
		FirstTS, LastTS         int64
	}
	err := tx.Select("CASE " + group + " WHEN '' THEN '-' ELSE " + group + " END AS \"group\", " + sel).Group(group).Scan(&rows).Error
	stats := make([]component.Stat, len(rows))
	for i, r := range rows {
		stats[i] = component.Stat{Group: r.Group, Family: r.Family, Runs: r.Runs, Optimized: r.Optimized, Failed: r.Failed,
			RawTokens: r.RawTokens, OutTokens: r.OutTokens, First: time.UnixMilli(r.FirstTS), Last: time.UnixMilli(r.LastTS)}
	}
	return stats, err
}

// mergeStats sums groups present in both lists and orders them as liveStats does.
func mergeStats(q component.StatsQuery, lists ...[]component.Stat) []component.Stat {
	byGroup := map[string]component.Stat{}
	for _, list := range lists {
		for _, s := range list {
			m, ok := byGroup[s.Group]
			if !ok {
				byGroup[s.Group] = s
				continue
			}
			m.Family = max(m.Family, s.Family)
			m.Runs += s.Runs
			m.Optimized += s.Optimized
			m.Failed += s.Failed
			m.RawTokens += s.RawTokens
			m.OutTokens += s.OutTokens
			if s.First.Before(m.First) {
				m.First = s.First
			}
			if s.Last.After(m.Last) {
				m.Last = s.Last
			}
			byGroup[s.Group] = m
		}
	}
	out := make([]component.Stat, 0, len(byGroup))
	for _, s := range byGroup {
		out = append(out, s)
	}
	slices.SortFunc(out, func(a, b component.Stat) int {
		if q.Chronological() {
			return cmp.Compare(a.Group, b.Group)
		}
		return cmp.Or(cmp.Compare(b.Saved(), a.Saved()), cmp.Compare(a.Group, b.Group))
	})
	return out
}
