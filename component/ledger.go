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
	Filter     string // filter that bypassed optimization, when Outcome is Bypassed
	Steps      []Step // optimizers that changed the payload, in order
}

// Step is one optimizer's effect on a payload.
type Step struct {
	Optimizer string
	Family    string
	RawTokens int // before the step
	OutTokens int // after the step
}

// Failure reasons.
const (
	ReasonError        = "error"
	ReasonRawRequested = "raw_requested"
	ReasonManual       = "manual"
)

// Stat aggregates events (or optimizer steps) of one group.
type Stat struct {
	Group     string
	Family    string // optimizer family, for ByOptimizer
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
	ByCommand   = "command"
	BySkip      = "skip"    // bypassed events by the filter that skipped them
	ByOutcome   = "outcome" // optimized, unchanged, bypassed, error
)

// StatsQuery selects what Stats aggregates.
type StatsQuery struct {
	By      string // ByRoute when empty
	Session string // all sessions when empty
}

func (s Stat) Saved() int { return s.RawTokens - s.OutTokens }

// Ledger is the extension contract the service relies on to keep originals
// retrievable and to learn which sources optimize badly.
type Ledger interface {
	SaveRaw(ctx context.Context, key string, p *payload.Payload) (id string, err error)
	GetRaw(ctx context.Context, id string) (key string, p *payload.Payload, err error)
	Record(ctx context.Context, ev Event) error
	MarkFailure(ctx context.Context, key, reason string) error
	Failures(ctx context.Context, key string) (int, error)
	// Stats aggregates recorded events, biggest savings first.
	Stats(ctx context.Context, q StatsQuery) ([]Stat, error)
}
