package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/internal/router"
	"github.com/lukaszb/gascoigne/payload"
)

// Pipeline runs filter → router → optimizers → exporters for one payload.
type Pipeline struct {
	name      string
	svc       *Service
	filters   []named[component.Filter]
	router    *router.Config
	exporters []named[component.Exporter]
}

func (pl *Pipeline) Process(ctx context.Context, in *payload.Payload) (*payload.Payload, error) {
	p, err := pl.Optimize(ctx, in)
	if err != nil {
		return nil, err
	}
	for _, e := range pl.exporters {
		if err := e.c.Export(ctx, p); err != nil {
			return nil, fmt.Errorf("exporter %s: %w", e.id, err)
		}
	}
	return p, nil
}

// Optimize runs everything but exporters. The input is never modified.
// Whenever output differs from input, the original is saved to the ledger
// first, so agents can always get the full payload back.
func (pl *Pipeline) Optimize(ctx context.Context, in *payload.Payload) (*payload.Payload, error) {
	start := time.Now()
	p := in.Clone()
	for k, v := range pl.svc.attrs {
		if _, ok := p.Attrs[k]; !ok {
			p.Set(k, v)
		}
	}
	ev := component.Event{TS: start, Pipeline: pl.name, Key: p.Key(), RawTokens: p.Tokens()}
	p, err := pl.run(ctx, p, &ev)
	if err != nil {
		return nil, err
	}
	ev.OutTokens = p.Tokens()
	ev.Duration = time.Since(start)
	p.Set(payload.AttrRawTokens, strconv.Itoa(ev.RawTokens))
	p.Set(payload.AttrOutTokens, strconv.Itoa(ev.OutTokens))
	pl.svc.log.Debug("processed", "pipeline", pl.name, "key", ev.Key, "route", ev.Route,
		"optimizers", ev.Optimizers, "outcome", ev.Outcome, "raw_tokens", ev.RawTokens, "out_tokens", ev.OutTokens)
	if l := pl.svc.ledger; l != nil {
		if err := l.Record(ctx, ev); err != nil {
			pl.svc.log.Warn("ledger record", "err", err)
		}
	}
	return p, nil
}

func (pl *Pipeline) run(ctx context.Context, p *payload.Payload, ev *component.Event) (*payload.Payload, error) {
	for _, f := range pl.filters {
		d, err := f.c.Filter(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", f.id, err)
		}
		if d == component.Bypass {
			pl.svc.log.Debug("bypass", "filter", f.id, "key", ev.Key)
			ev.Outcome = component.Bypassed
			return p, nil
		}
	}

	ev.Route, ev.Optimizers = pl.router.Route(p)
	p.Set(payload.AttrRoute, ev.Route)
	orig := p.Clone()
	for _, id := range ev.Optimizers {
		if err := pl.svc.optimizers[id].Optimize(ctx, p); err != nil {
			// Never fail the agent: fall back to the original and remember the source.
			pl.svc.log.Warn("optimizer failed, using original", "optimizer", id, "err", err)
			ev.Outcome = component.Failed
			pl.markFailure(ctx, ev.Key, component.ReasonError)
			return orig, nil
		}
	}
	if p.SameContent(orig) {
		ev.Outcome = component.Unchanged
		return p, nil
	}

	id, err := pl.svc.ledger.SaveRaw(ctx, ev.Key, orig)
	if err != nil {
		// Without a saved original the optimized output would be lossy.
		pl.svc.log.Warn("save raw failed, using original", "err", err)
		ev.Outcome = component.Failed
		return orig, nil
	}
	p.Set(payload.AttrRawID, id)
	ev.RawID = id
	ev.Outcome = component.Optimized
	return p, nil
}

func (pl *Pipeline) markFailure(ctx context.Context, key, reason string) {
	if key == "" || pl.svc.ledger == nil {
		return
	}
	if err := pl.svc.ledger.MarkFailure(ctx, key, reason); err != nil {
		pl.svc.log.Warn("ledger mark failure", "err", err)
	}
}
