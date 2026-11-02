// Package historyfilter bypasses sources whose optimization failed before,
// so agents are not repeatedly handed output they had to re-fetch.
package historyfilter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// AttrLevel is the payload attr the filter sets; a route matches it
// (`match: { attrs: { level: gentle } }`) to pick a less cutting chain.
const (
	AttrLevel   = "level"
	LevelGentle = "gentle"
)

type Filter struct {
	cfg    *Config
	log    *slog.Logger
	ledger component.Ledger
}

func (f *Filter) Start(_ context.Context, host component.Host) (err error) {
	f.ledger, err = mcptool.Ledger(host, mcptool.LedgerID)
	return err
}

func (f *Filter) Shutdown(context.Context) error { return nil }

func (f *Filter) Filter(ctx context.Context, p *payload.Payload) (component.Decision, error) {
	key := p.Key()
	if key == "" {
		return component.Continue, nil
	}
	info, err := f.ledger.Failures(ctx, key)
	if err != nil {
		return component.Continue, fmt.Errorf("failures %s: %w", key, err)
	}
	limit := f.limit(key, info)
	if info.Count >= limit && !f.expired(info) {
		f.log.Debug("known bad key", "key", key, "failures", info.Count)
		p.Set(payload.AttrSkip, fmt.Sprintf("%s failed %d× (max_failures %d), last: %s at %s; reset: gascoigne forget %q",
			key, info.Count, limit, info.Reason, info.Last.Format(time.DateTime), key))
		return component.Bypass, nil
	}
	if g := f.cfg.GentleAfter; g > 0 && info.Count >= g && !f.expired(info) {
		f.log.Debug("gentle key", "key", key, "failures", info.Count)
		p.Set(AttrLevel, LevelGentle)
	}
	return component.Continue, nil
}

// limit is the failure count that bypasses key: a whole read of a fetched
// page is how docs are meant to be read, so it takes more of them.
func (f *Filter) limit(key string, info component.FailureInfo) int {
	if strings.HasPrefix(key, "url:") && info.Reason == component.ReasonRawRequested {
		return f.cfg.FetchMaxFailures
	}
	return f.cfg.MaxFailures
}

// expired reports a last failure older than failure_ttl.
func (f *Filter) expired(info component.FailureInfo) bool {
	return f.cfg.FailureTTL > 0 && time.Since(info.Last) > f.cfg.FailureTTL
}
