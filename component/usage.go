package component

import (
	"context"
	"time"
)

// Usage is what one LLM API request was billed, as the provider reported
// it in the response. Input excludes cached input; CacheRead and CacheWrite
// are input tokens read from or written to the prompt cache.
type Usage struct {
	TS         time.Time
	Agent      string
	Model      string
	Input      int
	CacheRead  int
	CacheWrite int
	Output     int
}

// UsageRecorder is implemented by ledgers that keep API usage. Optional:
// components type-assert a Ledger for it.
type UsageRecorder interface {
	RecordUsage(ctx context.Context, u Usage) error
}
