// Package historyfilter bypasses sources whose optimization failed before,
// so agents are not repeatedly handed output they had to re-fetch.
package historyfilter

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/payload"
)

type Filter struct {
	cfg    *Config
	log    *slog.Logger
	ledger component.Ledger
}

func (f *Filter) Start(_ context.Context, host component.Host) error {
	id, _ := component.ParseID(f.cfg.Ledger)
	ext, ok := host.Extension(id)
	if !ok {
		return fmt.Errorf("extension %s not configured", id)
	}
	if f.ledger, ok = ext.(component.Ledger); !ok {
		return fmt.Errorf("extension %s is not a ledger", id)
	}
	return nil
}

func (f *Filter) Shutdown(context.Context) error { return nil }

func (f *Filter) Filter(ctx context.Context, p *payload.Payload) (component.Decision, error) {
	key := p.Key()
	if key == "" {
		return component.Continue, nil
	}
	n, err := f.ledger.Failures(ctx, key)
	if err != nil {
		return component.Continue, fmt.Errorf("failures %s: %w", key, err)
	}
	if n >= f.cfg.MaxFailures {
		f.log.Debug("known bad key", "key", key, "failures", n)
		return component.Bypass, nil
	}
	return component.Continue, nil
}
