package ledger

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

// grouping is a column the live stats group by, of the events table or, when
// steps is set, of the steps joined to their events.
type grouping struct {
	column string
	steps  bool
}

var groupings = map[string]grouping{
	component.ByRoute:    {column: "events.route"},
	component.BySession:  {column: "events.session"},
	component.ByAgent:    {column: "events.agent"},
	component.ByModel:    {column: "events.model"},
	component.ByCommand:  {column: "events.command"},
	component.ByPipeline: {column: "events.pipeline"},
	component.ByType:     {column: `events."type"`},
	component.ByOutcome:  {column: "events.outcome"},
	component.ByDay:      {column: period(component.ByDay)},
	component.ByWeek:     {column: period(component.ByWeek)},
	component.ByMonth:    {column: period(component.ByMonth)},

	component.ByFamily:    {column: "steps.family", steps: true},
	component.ByOptimizer: {column: "steps.optimizer", steps: true},
	component.ByDialect:   {column: "steps.dialect", steps: true},
}

// reports are the groupings with a query of their own.
var reports = map[string]func(ledger *Ledger, ctx context.Context, query component.StatsQuery) ([]component.Stat, error){
	component.ByReason:        (*Ledger).reasons,
	component.ByFamilyOutcome: (*Ledger).familyOutcomes,
}

// liveStats aggregates the events still stored by route or session, or
// optimizer steps by family or optimizer, optionally within one session;
// biggest savings first. Stats adds the rolled-up history to it.
func (l *Ledger) liveStats(ctx context.Context, query component.StatsQuery) ([]component.Stat, error) {
	by := cmp.Or(query.By, component.ByRoute)
	if report, ok := reports[by]; ok {
		return report(l, ctx, query)
	}
	g, ok := groupings[by]
	if !ok {
		return nil, fmt.Errorf("stats: unknown grouping %q", query.By)
	}
	tx, table := l.events(ctx), "events"
	if g.steps {
		tx, table = l.steps(ctx), "steps"
	}
	group := g.column
	tx = filter(tx, "events", query)
	var rows []statRow
	err := tx.Select("CASE " + group + " WHEN '' THEN '-' ELSE " + group + " END AS \"group\", " + l.aggregates(query.By)).
		Group(group).
		Order(l.liveOrder(table, query)).
		Scan(&rows).Error
	stats := make([]component.Stat, len(rows))
	for i, row := range rows {
		stats[i] = row.stat()
	}
	return stats, err
}

// liveOrder is the ORDER BY of live stats: biggest savings first, oldest first for a time
// grouping, latest session first for sessions.
func (l *Ledger) liveOrder(table string, query component.StatsQuery) string {
	switch {
	case query.By == component.BySession:
		return "max(events.ts) DESC, 1"
	case query.Chronological():
		return "1"
	}
	return fmt.Sprintf("sum(%[1]s.raw_tokens) - sum(%[2]s) DESC, 1", table, l.outTokens(query.By))
}

// statRow is one aggregated row of live stats, as the database returns it.
type statRow struct {
	Group                   string
	Family                  string
	Runs, Optimized, Failed int
	RawTokens, OutTokens    int
	FirstTS, LastTS         int64
}

func (row statRow) stat() component.Stat {
	return component.Stat{Group: row.Group, Family: row.Family, Runs: row.Runs, Optimized: row.Optimized, Failed: row.Failed,
		RawTokens: row.RawTokens, OutTokens: row.OutTokens, First: time.UnixMilli(row.FirstTS), Last: time.UnixMilli(row.LastTS)}
}

// period buckets events.ts (unix ms) in local time.
func period(by string) string {
	format := map[string]string{component.ByDay: "%Y-%m-%d", component.ByWeek: "%Y-W%W", component.ByMonth: "%Y-%m"}[by]
	return fmt.Sprintf("strftime('%s', events.ts / 1000, 'unixepoch', 'localtime')", format)
}

// filter applies the query's session, agent, model, pipeline and time filters to an
// events table (or alias).
func filter(tx *gorm.DB, events string, query component.StatsQuery) *gorm.DB {
	for col, v := range map[string]string{"session": query.Session, "agent": query.Agent, "model": query.Model} {
		if v != "" {
			tx = tx.Where(events+"."+col+" = ?", v)
		}
	}
	if names := pipelineNames(query.Pipelines); len(names) > 0 {
		tx = tx.Where(events+".pipeline IN ?", names)
	}
	if !query.Since.IsZero() {
		tx = tx.Where(events+".ts >= ?", query.Since.UnixMilli())
	}
	return tx
}

// pipelineNames trims the filter's names and drops empty ones.
func pipelineNames(names []string) []string {
	var out []string
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func (l *Ledger) events(ctx context.Context) *gorm.DB {
	return l.db.WithContext(ctx).Model(&event{})
}

func (l *Ledger) steps(ctx context.Context) *gorm.DB {
	return l.db.WithContext(ctx).Model(&step{}).Joins("JOIN events ON events.id = steps.event_id")
}

// eventOut is the tokens an event's run cost the agent. A rerequested run
// cost the optimized result and then the original too: out + raw tokens
// where it would have read raw.
const eventOut = "CASE WHEN events.outcome = 'rerequested' THEN events.out_tokens + events.raw_tokens ELSE events.out_tokens END"

// outTokens is the column or expression the grouping sums as output tokens.
func (l *Ledger) outTokens(by string) string {
	if by == component.ByFamily || by == component.ByOptimizer || by == component.ByDialect {
		return "steps.out_tokens"
	}
	return eventOut
}

// aggregates: events count outcomes; every step is an optimization.
func (l *Ledger) aggregates(by string) string {
	if by == component.ByFamily || by == component.ByOptimizer || by == component.ByDialect {
		return `max(steps.family) AS family, count(*) AS runs, count(*) AS optimized, 0 AS failed,
			sum(steps.raw_tokens) AS raw_tokens, sum(steps.out_tokens) AS out_tokens,
			min(events.ts) AS first_ts, max(events.ts) AS last_ts`
	}
	return `count(*) AS runs, sum(outcome = 'optimized') AS optimized, sum(outcome = 'error') AS failed,
		sum(raw_tokens) AS raw_tokens,
		sum(` + eventOut + `) AS out_tokens,
		min(ts) AS first_ts, max(ts) AS last_ts`
}

type Failure struct {
	Key    string
	Count  int
	Last   time.Time
	Reason string
	// RawTokens passed unoptimized because the history filter bypassed the
	// key since its last failure.
	RawTokens int
}

func (l *Ledger) FailureList(ctx context.Context) ([]Failure, error) {
	var rows []failure
	if err := l.db.WithContext(ctx).Order("last_ts DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	fs := make([]Failure, len(rows))
	for i, r := range rows {
		fs[i] = Failure{Key: r.Key, Count: r.Count, Last: time.UnixMilli(r.LastTS), Reason: r.Reason}
		err := l.db.WithContext(ctx).Model(&event{}).
			Where("key = ? AND filter = 'history' AND outcome = ? AND ts >= ?", r.Key, component.Bypassed, r.LastTS).
			Select("coalesce(sum(raw_tokens), 0)").Scan(&fs[i].RawTokens).Error
		if err != nil {
			return nil, err
		}
	}
	return fs, nil
}

// reasons groups runs that were not optimized by outcome, stopping component
// and command, each with its latest reason.
func (l *Ledger) reasons(ctx context.Context, query component.StatsQuery) ([]component.Stat, error) {
	var rows []struct {
		Outcome, Filter, Command   string
		Runs, RawTokens, OutTokens int
		FirstTS, LastTS            int64
		LastID                     uint
	}
	tx := filter(l.db.WithContext(ctx).Table("events e"), "e", query).
		Where("e.outcome NOT IN ?", []string{string(component.Optimized), string(component.Rerequested)})
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
func (l *Ledger) Recent(ctx context.Context, query component.StatsQuery, after uint, limit int) ([]Entry, error) {
	return l.Page(ctx, query, Page{After: after, Limit: limit})
}

// Page selects a slice of the run feed, newest first.
type Page struct {
	After  uint // only runs with a larger id (new ones, for a live feed)
	Before uint // only runs with a smaller id (older ones, loading on scroll); 0 = no bound
	// Outcomes keeps runs with these outcomes ("optimized", "unchanged",
	// "bypassed", "error"); empty = all.
	Outcomes []string
	// RawAsked keeps runs where the agent asked for the original straight
	// away: skipped with a "raw requested" reason (-raw, MCP raw,
	// GASCOIGNE_RAW) and no earlier optimized run of that source in the session.
	RawAsked bool
	Limit    int
}

// rawRequested matches the reason of a run skipped because the agent asked
// for the original.
const rawRequested = "raw requested%"

// earlierOptimized finds an optimized run (since rerequested) of the same
// source and session before the one being filtered.
const earlierOptimized = "SELECT 1 FROM events e2 WHERE e2.key = events.key AND e2.session = events.session " +
	"AND e2.outcome IN ('optimized', 'rerequested') AND e2.id < events.id"

// Page returns the runs matching q and p, newest first.
func (l *Ledger) Page(ctx context.Context, query component.StatsQuery, page Page) ([]Entry, error) {
	var rows []event
	tx := filter(l.db.WithContext(ctx).Preload("Steps"), "events", query).Where("events.id > ?", page.After)
	if page.Before > 0 {
		tx = tx.Where("events.id < ?", page.Before)
	}
	if len(page.Outcomes) > 0 {
		tx = tx.Where("events.outcome IN ?", page.Outcomes)
	}
	if page.RawAsked {
		tx = tx.Where("events.reason LIKE ? AND NOT EXISTS ("+earlierOptimized+")", rawRequested)
	}
	if err := tx.Order("events.id DESC").Limit(page.Limit).Find(&rows).Error; err != nil {
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
		RawID: r.RawID, Outcome: component.Outcome(r.Outcome), Filter: r.Filter, Reason: r.Reason, Type: r.Type, Steps: steps,
	}
}

// Reset deletes recorded events and their steps matching q's filters and
// reports how many events went. all also deletes stored originals and
// failure history, which carry no session or agent, so it takes no filters.
func (l *Ledger) Reset(ctx context.Context, query component.StatsQuery, all bool) (int64, error) {
	if all && (query.Session != "" || query.Agent != "" || query.Model != "" || len(pipelineNames(query.Pipelines)) > 0 || !query.Since.IsZero()) {
		return 0, errors.New("reset: originals and failure history cannot be filtered; drop the filters or the full reset")
	}
	var n int64
	err := l.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := filter(tx.Model(&event{}).Select("id"), "events", query)
		if err := tx.Where("event_id IN (?)", ids).Delete(&step{}).Error; err != nil {
			return err
		}
		res := filter(tx.Where("1 = 1"), "events", query).Delete(&event{})
		if res.Error != nil {
			return res.Error
		}
		n = res.RowsAffected
		if query.Session == "" && len(pipelineNames(query.Pipelines)) == 0 { // usage has no session or pipeline
			if err := usageFilter(tx.Where("1 = 1"), query).Delete(&usage{}).Error; err != nil {
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

// Run is one shortened run whose original can still be read.
type Run struct {
	Key       string `json:"key"`
	RawID     string `json:"raw_id"`
	RawTokens int    `json:"raw_tokens"`
	OutTokens int    `json:"out_tokens"`
}

// RecentRuns lists the newest optimized runs of a session that kept an
// original, one per source key (the latest), newest first, at most limit.
func (l *Ledger) RecentRuns(ctx context.Context, session string, limit int) ([]Run, error) {
	var rows []Run
	err := l.db.WithContext(ctx).Model(&event{}).
		Select("key, raw_id, raw_tokens, out_tokens").
		Where("session = ? AND raw_id != '' AND outcome IN ?", session, []string{string(component.Optimized), string(component.Rerequested)}).
		Order("id DESC").Limit(limit * 20).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	var out []Run
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.Key] {
			continue
		}
		seen[r.Key] = true
		if out = append(out, r); len(out) == limit {
			break
		}
	}
	return out, nil
}

// ImpactDay compares, for one day, what optimizing saved with what
// rerequests gave back.
type ImpactDay struct {
	Day         string `json:"day"`
	Optimized   int    `json:"optimized"`   // runs the agent was content with
	Saved       int    `json:"saved"`       // tokens those runs saved
	Rerequested int    `json:"rerequested"` // runs after which the agent asked for the original
	Wasted      int    `json:"wasted"`      // tokens the agent read twice for them: the optimized result on top of the original
}

// Impact is the daily optimized-versus-rerequested comparison, oldest first.
func (l *Ledger) Impact(ctx context.Context, query component.StatsQuery) ([]ImpactDay, error) {
	var days []ImpactDay
	err := filter(l.events(ctx), "events", query).
		Where("events.outcome IN ?", []string{string(component.Optimized), string(component.Rerequested)}).
		Select(period(component.ByDay) + ` AS day,
			sum(outcome = 'optimized') AS optimized,
			sum(CASE WHEN outcome = 'optimized' THEN raw_tokens - out_tokens ELSE 0 END) AS saved,
			sum(outcome = 'rerequested') AS rerequested,
			sum(CASE WHEN outcome = 'rerequested' THEN out_tokens ELSE 0 END) AS wasted`).
		Group("day").Order("day").Scan(&days).Error
	return days, err
}

// familyOutcomes counts the runs per optimizer family and outcome: a run is
// in every family one of its steps belongs to.
func (l *Ledger) familyOutcomes(ctx context.Context, query component.StatsQuery) ([]component.Stat, error) {
	var rows []struct {
		Family, Outcome            string
		Runs, RawTokens, OutTokens int
		FirstTS, LastTS            int64
	}
	tx := filter(l.steps(ctx), "events", query)
	err := tx.Select(`steps.family AS family, events.outcome AS outcome, count(DISTINCT events.id) AS runs,
			sum(events.raw_tokens) AS raw_tokens, sum(events.out_tokens) AS out_tokens,
			min(events.ts) AS first_ts, max(events.ts) AS last_ts`).
		Group("steps.family, events.outcome").
		Order("steps.family, events.outcome").
		Scan(&rows).Error
	stats := make([]component.Stat, len(rows))
	for i, r := range rows {
		stats[i] = component.Stat{Group: r.Family, Family: r.Family, Outcome: r.Outcome, Runs: r.Runs,
			RawTokens: r.RawTokens, OutTokens: r.OutTokens, First: time.UnixMilli(r.FirstTS), Last: time.UnixMilli(r.LastTS)}
	}
	return stats, err
}
