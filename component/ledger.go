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
	Pipeline   string
	Key        string
	Route      string
	Optimizers []string
	RawTokens  int
	OutTokens  int
	Duration   time.Duration
	RawID      string
	Outcome    Outcome
}

// Failure reasons.
const (
	ReasonError        = "error"
	ReasonRawRequested = "raw_requested"
	ReasonManual       = "manual"
)

// Ledger is the extension contract the service relies on to keep originals
// retrievable and to learn which sources optimize badly.
type Ledger interface {
	SaveRaw(ctx context.Context, key string, p *payload.Payload) (id string, err error)
	GetRaw(ctx context.Context, id string) (key string, p *payload.Payload, err error)
	Record(ctx context.Context, ev Event) error
	MarkFailure(ctx context.Context, key, reason string) error
	Failures(ctx context.Context, key string) (int, error)
}
