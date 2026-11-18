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

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/router"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// minSaving is the fewest tokens an optimization must save to be used: the
// "[raw:<id>]" hint costs
// about this much, so saving less would cost the agent tokens.
const minSaving = 20

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
	if pl.svc.remote != nil {
		return pl.svc.remote(ctx, pl.name, p)
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

func (pl *Pipeline) run(ctx context.Context, data *payload.Payload, ev *component.Event) (*payload.Payload, error) {
	bypassed, err := pl.runFilters(ctx, data, ev)
	if err != nil || bypassed {
		return data, err
	}
	ev.Type = detectedTypes(data) // before optimizers rewrite block types

	ev.Route, ev.Optimizers = pl.router.Route(data)
	data.Set(payload.AttrRoute, ev.Route)
	orig := data.Clone()
	if failed := pl.runOptimizers(ctx, data, ev); failed {
		return orig, nil
	}
	return pl.keepIfWorthIt(ctx, data, orig, ev), nil
}

// runFilters runs the filters in order; true when one bypassed the payload (the event says which).
func (pl *Pipeline) runFilters(ctx context.Context, data *payload.Payload, ev *component.Event) (bool, error) {
	for _, f := range pl.filters {
		d, err := f.c.Filter(ctx, data)
		if err != nil {
			return false, fmt.Errorf("filter %s: %w", f.id, err)
		}
		if d == component.Bypass {
			pl.svc.log.Debug("bypass", "filter", f.id, "key", ev.Key)
			ev.Outcome, ev.Filter = component.Bypassed, f.id
			ev.Reason = cmp.Or(data.Attrs[payload.AttrSkip], "bypassed by "+f.id)
			ev.Type = detectedTypes(data)
			return true, nil
		}
	}
	return false, nil
}

// runOptimizers applies the routed optimizers and records a step for each that changed the payload.
// True when one failed: the caller then uses the original.
func (pl *Pipeline) runOptimizers(ctx context.Context, data *payload.Payload, ev *component.Event) bool {
	for _, id := range ev.Optimizers {
		before, texts := data.Tokens(), blockTexts(data)
		if err := pl.svc.optimizers[id].Optimize(ctx, data); err != nil {
			// Never fail the agent: fall back to the original and remember the source.
			pl.svc.log.Warn("optimizer failed, using original", "optimizer", id, "err", err)
			ev.Outcome, ev.Steps, ev.Filter, ev.Reason = component.Failed, nil, id, firstLine(err.Error())
			pl.markFailure(ctx, ev.Key, component.ReasonError)
			return true
		}
		if after := data.Tokens(); after != before {
			ev.Steps = append(ev.Steps, component.Step{Optimizer: id, Family: pl.svc.families[id],
				Dialect: changedTypes(data, texts), RawTokens: before, OutTokens: after})
		}
	}
	return false
}

// keepIfWorthIt returns the optimized payload after saving the original, or the original when
// nothing changed, the saving is too small or the original could not be saved.
func (pl *Pipeline) keepIfWorthIt(ctx context.Context, data, orig *payload.Payload, ev *component.Event) *payload.Payload {
	if data.SameContent(orig) {
		ev.Outcome, ev.Reason = component.Unchanged, unchangedReason(data, ev.Optimizers)
		return data
	}
	if in, out := orig.Tokens(), data.Tokens(); in-out < minSaving {
		// Rewritten but not worth it: the retrieve hint every consumer adds
		// costs about minSaving tokens, and the original reads better.
		ev.Outcome, ev.Steps = component.Unchanged, nil
		ev.Reason = fmt.Sprintf("saved %d tokens (%d → %d) < %d, the cost of the retrieve hint: original kept", in-out, in, out, minSaving)
		return orig
	}

	id, err := pl.svc.ledger.SaveRaw(ctx, ev.Key, orig)
	if err != nil {
		// Without a saved original the optimized output would be lossy.
		pl.svc.log.Warn("save raw failed, using original", "err", err)
		ev.Outcome, ev.Steps, ev.Filter, ev.Reason = component.Failed, nil, "ledger", "save original: "+firstLine(err.Error())
		return orig
	}
	data.Set(payload.AttrRawID, id)
	ev.RawID = id
	ev.Outcome = component.Optimized
	ev.Output = data.Text()
	return data
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
func command(data *payload.Payload) string {
	if c := detect.Command(data.Attrs[payload.AttrCmd]); c != "" {
		return c
	}
	tool := data.Attrs[payload.AttrTool]
	if u, err := url.Parse(data.Attrs[payload.AttrURL]); err == nil && u.Host != "" {
		return strings.TrimSpace(tool + " " + u.Host)
	}
	return tool
}

// unchangedReason tells which content no optimizer handled.
func unchangedReason(data *payload.Payload, optimizers []string) string {
	if len(optimizers) == 0 {
		return "route has no optimizers"
	}
	var types []string
	for _, b := range data.Blocks {
		types = append(types, cmp.Or(b.Type, "undetected"))
	}
	reason := "no optimizer changed it · types: " + strings.Join(slices.Compact(types), ", ")
	if note := data.Attrs[payload.AttrSkip]; note != "" {
		reason = note + " · types: " + strings.Join(slices.Compact(types), ", ")
	}
	return reason
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return line
}

func blockTexts(data *payload.Payload) []string {
	texts := make([]string, len(data.Blocks))
	for i, b := range data.Blocks {
		texts[i] = b.Text
	}
	return texts
}

// changedTypes lists the types of blocks whose text differs from before.
func changedTypes(data *payload.Payload, before []string) string {
	var types []string
	for i, b := range data.Blocks {
		if i >= len(before) || b.Text != before[i] {
			types = append(types, cmp.Or(b.Type, "-"))
		}
	}
	return joinTypes(types)
}

// detectedTypes lists the types detected for the payload's blocks, same shape
// as changedTypes; empty when no block has one.
func detectedTypes(data *payload.Payload) string {
	var types []string
	for _, b := range data.Blocks {
		if b.Type != "" {
			types = append(types, b.Type)
		}
	}
	return joinTypes(types)
}

// joinTypes sorts the types, drops repeats and joins them with "+".
func joinTypes(types []string) string {
	slices.Sort(types)
	return strings.Join(slices.Compact(types), "+")
}
