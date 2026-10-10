package service

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
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
	ev := component.Event{TS: start, Session: p.Attrs[payload.AttrSession], Agent: p.Attrs[payload.AttrAgent], Model: p.Attrs[payload.AttrModel], Command: command(p), Pipeline: pl.name, Key: p.Key(), RawTokens: p.Tokens()}
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
			ev.Outcome, ev.Filter = component.Bypassed, f.id
			ev.Reason = cmp.Or(p.Attrs[payload.AttrSkip], "bypassed by "+f.id)
			return p, nil
		}
	}

	ev.Route, ev.Optimizers = pl.router.Route(p)
	p.Set(payload.AttrRoute, ev.Route)
	orig := p.Clone()
	for _, id := range ev.Optimizers {
		before := p.Tokens()
		if err := pl.svc.optimizers[id].Optimize(ctx, p); err != nil {
			// Never fail the agent: fall back to the original and remember the source.
			pl.svc.log.Warn("optimizer failed, using original", "optimizer", id, "err", err)
			ev.Outcome, ev.Steps, ev.Filter, ev.Reason = component.Failed, nil, id, firstLine(err.Error())
			pl.markFailure(ctx, ev.Key, component.ReasonError)
			return orig, nil
		}
		if after := p.Tokens(); after != before {
			ev.Steps = append(ev.Steps, component.Step{Optimizer: id, Family: pl.svc.families[id], RawTokens: before, OutTokens: after})
		}
	}
	if p.SameContent(orig) {
		ev.Outcome, ev.Reason = component.Unchanged, unchangedReason(p, ev.Optimizers)
		return p, nil
	}

	id, err := pl.svc.ledger.SaveRaw(ctx, ev.Key, orig)
	if err != nil {
		// Without a saved original the optimized output would be lossy.
		pl.svc.log.Warn("save raw failed, using original", "err", err)
		ev.Outcome, ev.Steps, ev.Filter, ev.Reason = component.Failed, nil, "ledger", "save original: "+firstLine(err.Error())
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

// command names the payload's source for reports: the normalized command
// line, else the tool (and URL host), else empty.
func command(p *payload.Payload) string {
	if c := detect.Command(p.Attrs[payload.AttrCmd]); c != "" {
		return c
	}
	tool := p.Attrs[payload.AttrTool]
	if u, err := url.Parse(p.Attrs[payload.AttrURL]); err == nil && u.Host != "" {
		return strings.TrimSpace(tool + " " + u.Host)
	}
	return tool
}

// unchangedReason tells which content no optimizer handled.
func unchangedReason(p *payload.Payload, optimizers []string) string {
	if len(optimizers) == 0 {
		return "route has no optimizers"
	}
	var types []string
	for _, b := range p.Blocks {
		types = append(types, cmp.Or(b.Type, "undetected"))
	}
	reason := "no optimizer changed it · types: " + strings.Join(slices.Compact(types), ", ")
	if note := p.Attrs[payload.AttrSkip]; note != "" {
		reason = note + " · types: " + strings.Join(slices.Compact(types), ", ")
	}
	return reason
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
