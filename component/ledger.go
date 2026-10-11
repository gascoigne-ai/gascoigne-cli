package component

import (
	"context"
	"time"

	"github.com/lukaszb/gascoigne/payload"
)

type Outcome string

const (
	Optimized Outcome = "optimized"
	Unchanged Outcome = "unchanged"
	Bypassed  Outcome = "bypassed"
	Failed    Outcome = "error"
)

// Event is one payload's trip through a pipeline.
type Event struct {
	TS         time.Time
	Session    string // agent session, from the session attr
	Agent      string // agent name (claude-code, codex, …), from the agent attr
	Model      string // LLM model the output is for, from the model attr
	Command    string // what produced the payload, for reports ("go test", "fetch go.dev")
	Pipeline   string
	Key        string
	Route      string
	Optimizers []string
	RawTokens  int
	OutTokens  int
	Duration   time.Duration
	RawID      string
	Outcome    Outcome
	Filter     string // component that stopped optimization: bypassing filter, failing optimizer, "ledger"
	Reason     string // why it was not optimized: filter detail, optimizer error, unhandled types
	Steps      []Step // optimizers that changed the payload, in order
	// Output is the optimized text of an optimized run; ledgers may keep it
	// next to the original (RawID) so both can be compared.
	Output string
}

// Step is one optimizer's effect on a payload.
type Step struct {
	Optimizer string
	Family    string
	Dialect   string // content type(s) of the blocks it changed, e.g. "cloud/aws"
	RawTokens int    // before the step
	OutTokens int    // after the step
}

// Failure reasons.
const (
	ReasonError        = "error"
	ReasonRawRequested = "raw_requested"
	ReasonManual       = "manual"
)

// FailureInfo summarizes a source's failure history.
type FailureInfo struct {
	Count  int
	Reason string
	Last   time.Time
}

// Stat aggregates events (or optimizer steps) of one group.
type Stat struct {
	Group     string
	Family    string // optimizer family, for ByOptimizer
	Command   string // for ByReason
	Outcome   string // for ByReason
	Reason    string // latest reason, for ByReason
	Runs      int
	Optimized int
	Failed    int
	RawTokens int
	OutTokens int
	First     time.Time
	Last      time.Time
}

// Stats groupings.
const (
	ByRoute     = "route"
	BySession   = "session"
	ByFamily    = "family"    // optimizer family, from steps
	ByOptimizer = "optimizer" // single optimizer, from steps
	ByDialect   = "dialect"   // content type the optimizers changed, from steps
	ByCommand   = "command"
	ByOutcome   = "outcome" // optimized, unchanged, bypassed, error
	ByAgent     = "agent"
	ByModel     = "model"
	ByDay       = "day"   // local time, YYYY-MM-DD
	ByWeek      = "week"  // ISO-like YYYY-Www (weeks start Monday)
	ByMonth     = "month" // YYYY-MM
	// ByReason groups runs that were not optimized by outcome, filter (or
	// optimizer for errors) and command, with the latest reason.
	ByReason = "reason"
)

// StatsQuery selects what Stats aggregates.
type StatsQuery struct {
	By      string // ByRoute when empty
	Session string // filters; empty = all
	Agent   string
	Model   string
	Since   time.Time // zero = all time
}

// Chronological reports time groupings, which sort by time, not savings.
func (q StatsQuery) Chronological() bool {
	return q.By == ByDay || q.By == ByWeek || q.By == ByMonth
}

func (s Stat) Saved() int { return s.RawTokens - s.OutTokens }

// Ledger is the extension contract the service relies on to keep originals
// retrievable and to learn which sources optimize badly.
type Ledger interface {
	SaveRaw(ctx context.Context, key string, p *payload.Payload) (id string, err error)
	GetRaw(ctx context.Context, id string) (key string, p *payload.Payload, err error)
	Record(ctx context.Context, ev Event) error
	MarkFailure(ctx context.Context, key, reason string) error
	// Failures reports how often key optimized badly; zero Count when never.
	Failures(ctx context.Context, key string) (FailureInfo, error)
	// Stats aggregates recorded events, biggest savings first.
	Stats(ctx context.Context, q StatsQuery) ([]Stat, error)
}

// OutputHistory is implemented by ledgers that keep optimized outputs: what
// an agent was shown for a source, so a re-run can point at it.
type OutputHistory interface {
	// RecentOutputs returns outputs of the latest optimized runs of key in
	// session since the given time, newest first, at most limit.
	RecentOutputs(ctx context.Context, session, key string, since time.Time, limit int) ([]string, error)
}
