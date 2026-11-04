package inproc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// ErrNoLedger means the engine runs without a ledger, so there is nothing to read.
var ErrNoLedger = errors.New("engine has no ledger")

// Groupings StatsQuery.By accepts; empty means GroupRoute.
const (
	GroupRoute     = component.ByRoute
	GroupSession   = component.BySession
	GroupFamily    = component.ByFamily
	GroupOptimizer = component.ByOptimizer
	GroupCommand   = component.ByCommand
	GroupOutcome   = component.ByOutcome
	GroupAgent     = component.ByAgent
	GroupModel     = component.ByModel
	GroupDay       = component.ByDay
	GroupWeek      = component.ByWeek
	GroupMonth     = component.ByMonth
)

var groupings = map[string]bool{
	"": true, GroupRoute: true, GroupSession: true, GroupFamily: true, GroupOptimizer: true,
	GroupCommand: true, GroupOutcome: true, GroupAgent: true, GroupModel: true, GroupDay: true,
	GroupWeek: true, GroupMonth: true,
}

// StatsQuery selects what Stats totals. Empty filters match everything.
type StatsQuery struct {
	By      string    `json:"by,omitempty"`
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Model   string    `json:"model,omitempty"`
	Since   time.Time `json:"since,omitzero"`
}

// Stat totals the runs of one group.
type Stat struct {
	Group     string    `json:"group"`
	Runs      int       `json:"runs"`
	Optimized int       `json:"optimized"`
	Failed    int       `json:"failed"`
	RawTokens int       `json:"raw_tokens"`
	OutTokens int       `json:"out_tokens"`
	Saved     int       `json:"saved"`
	First     time.Time `json:"first,omitzero"`
	Last      time.Time `json:"last,omitzero"`
}

// Stats totals recorded runs per group. The wire types live here, not in component/, so the
// public contracts stay free of JSON tags.
func (e *Engine) Stats(ctx context.Context, q StatsQuery) ([]Stat, error) {
	return StatsOf(ctx, e.led, q)
}

// ErrBadQuery wraps a query the ledger is never asked, such as an unknown grouping.
var ErrBadQuery = errors.New("bad query")

// StatsOf is Stats over any ledger, for servers that own one.
func StatsOf(ctx context.Context, led *ledger.Ledger, q StatsQuery) ([]Stat, error) {
	if led == nil {
		return nil, ErrNoLedger
	}
	if !groupings[q.By] {
		return nil, fmt.Errorf("%w: unknown grouping %q", ErrBadQuery, q.By)
	}
	rows, err := led.Stats(ctx, q.component())
	if err != nil {
		return nil, err
	}
	out := make([]Stat, len(rows))
	for i, row := range rows {
		out[i] = Stat{
			Group: row.Group, Runs: row.Runs, Optimized: row.Optimized, Failed: row.Failed,
			RawTokens: row.RawTokens, OutTokens: row.OutTokens, Saved: row.Saved(),
			First: row.First, Last: row.Last,
		}
	}
	return out, nil
}

func (q StatsQuery) component() component.StatsQuery {
	return component.StatsQuery{By: q.By, Session: q.Session, Agent: q.Agent, Model: q.Model, Since: q.Since}
}

// DefaultRecentLimit and MaxRecentLimit bound Recent.
const (
	DefaultRecentLimit = 50
	MaxRecentLimit     = 1000
)

// RecentQuery selects recorded runs, newest first. Limit 0 means DefaultRecentLimit.
type RecentQuery struct {
	Session string    `json:"session,omitempty"`
	Agent   string    `json:"agent,omitempty"`
	Model   string    `json:"model,omitempty"`
	Since   time.Time `json:"since,omitzero"`
	// Before returns only runs with a smaller ID, to page back through older ones.
	Before uint `json:"before,omitempty"`
	Limit  int  `json:"limit,omitempty"`
}

// Run is one recorded optimization.
type Run struct {
	ID         uint          `json:"id"`
	Time       time.Time     `json:"time"`
	Session    string        `json:"session,omitempty"`
	Agent      string        `json:"agent,omitempty"`
	Model      string        `json:"model,omitempty"`
	Command    string        `json:"command,omitempty"`
	Route      string        `json:"route,omitempty"`
	Optimizers []string      `json:"optimizers,omitempty"`
	RawTokens  int           `json:"raw_tokens"`
	OutTokens  int           `json:"out_tokens"`
	Duration   time.Duration `json:"duration_ns"`
	RawID      string        `json:"raw_id,omitempty"`
	Outcome    string        `json:"outcome"`
	Reason     string        `json:"reason,omitempty"`
}

// Recent lists recorded runs, newest first.
func (e *Engine) Recent(ctx context.Context, q RecentQuery) ([]Run, error) {
	return RecentOf(ctx, e.led, q)
}

// RecentOf is Recent over any ledger, for servers that own one.
func RecentOf(ctx context.Context, led *ledger.Ledger, q RecentQuery) ([]Run, error) {
	if led == nil {
		return nil, ErrNoLedger
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultRecentLimit
	}
	limit = min(limit, MaxRecentLimit)
	filter := component.StatsQuery{Session: q.Session, Agent: q.Agent, Model: q.Model, Since: q.Since}
	rows, err := led.Page(ctx, filter, ledger.Page{Before: q.Before, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Run, len(rows))
	for i, row := range rows {
		out[i] = Run{
			ID: row.ID, Time: row.TS, Session: row.Session, Agent: row.Agent, Model: row.Model,
			Command: row.Command, Route: row.Route, Optimizers: row.Optimizers,
			RawTokens: row.RawTokens, OutTokens: row.OutTokens, Duration: row.Duration,
			RawID: hideHidden(row.RawID, row.Key), Outcome: string(row.Outcome), Reason: row.Reason,
		}
	}
	return out, nil
}

// hideHidden drops the raw id of instruction text, which Raw refuses to hand back.
func hideHidden(rawID, key string) string {
	if len(key) >= len(hiddenPrefix) && key[:len(hiddenPrefix)] == hiddenPrefix {
		return ""
	}
	return rawID
}
