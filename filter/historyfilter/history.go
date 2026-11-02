// Package historyfilter bypasses sources whose optimization failed before,
// so agents are not repeatedly handed output they had to re-fetch.
package historyfilter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
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
	now    func() time.Time

	mu       sync.Mutex
	rated    time.Time       // when gentleFamilies was read
	gentleFs map[string]bool // families fetched raw too often
}

func (f *Filter) Start(_ context.Context, host component.Host) (err error) {
	f.ledger, err = mcptool.Ledger(host, mcptool.LedgerID)
	return err
}

func (f *Filter) Shutdown(context.Context) error { return nil }

func (f *Filter) Filter(ctx context.Context, p *payload.Payload) (component.Decision, error) {
	if fam, err := f.gentleFamily(ctx, p); err != nil {
		return component.Continue, err
	} else if fam != "" {
		f.log.Debug("gentle family", "family", fam)
		p.Set(AttrLevel, LevelGentle)
	}
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

// familyRefresh is how long the family rates are reused.
const familyRefresh = time.Minute

// gentleFamily reports a block type whose family the agent fetches raw too
// often (family_rerequest_rate), from the ledger, read once a minute.
func (f *Filter) gentleFamily(ctx context.Context, p *payload.Payload) (string, error) {
	if f.cfg.FamilyRerequestRate <= 0 {
		return "", nil
	}
	fams, err := f.rawHeavyFamilies(ctx)
	if err != nil {
		return "", err
	}
	for _, b := range p.Blocks {
		if fam, _, _ := strings.Cut(b.Type, "/"); fams[fam] {
			return fam, nil
		}
	}
	return "", nil
}

func (f *Filter) rawHeavyFamilies(ctx context.Context) (map[string]bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	if f.gentleFs != nil && now.Sub(f.rated) < familyRefresh {
		return f.gentleFs, nil
	}
	stats, err := f.ledger.Stats(ctx, component.StatsQuery{By: component.ByFamilyOutcome, Since: now.Add(-f.cfg.FamilyWindow)})
	if err != nil {
		return nil, fmt.Errorf("family stats: %w", err)
	}
	runs := map[string]map[string]int{}
	for _, s := range stats {
		if runs[s.Family] == nil {
			runs[s.Family] = map[string]int{}
		}
		runs[s.Family][s.Outcome] += s.Runs
	}
	f.gentleFs = map[string]bool{}
	for fam, by := range runs {
		raw, all := by[string(component.Rerequested)], by[string(component.Optimized)]+by[string(component.Rerequested)]
		if all >= f.cfg.FamilyMinRuns && float64(raw) >= f.cfg.FamilyRerequestRate*float64(all) {
			f.gentleFs[fam] = true
		}
	}
	f.rated = now
	return f.gentleFs, nil
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
