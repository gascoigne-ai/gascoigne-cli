package main

import (
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"strings"
)

// priceOf looks a model up as transcripts name it: gateways add a provider
// prefix ("anthropic/claude-sonnet-5.5") and write versions with a dot.
func priceOf(model string) (cost.Model, bool) {
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:]
	}
	return cost.Lookup(strings.ReplaceAll(model, ".", "-"))
}

func compacted(prev, ctx int) bool {
	return prev >= minContext && float64(ctx) < compactDrop*float64(prev)
}

// cost prices the request as the transcript recorded it: cache writes at the
// 5-minute or the 1-hour rate it was made with.
func (t turn) cost(model cost.Model) float64 {
	return (float64(t.input)*model.Input + float64(t.read)*model.CacheRead + float64(t.write-t.write1h)*model.CacheWrite +
		float64(t.write1h)*oneHourWrite*model.Input + float64(t.output)*model.Output) / 1e6
}

// oneHourWrite is what a 1-hour cache write costs, in times the input price
// (a 5-minute write is 1.25 times).
const oneHourWrite = 2

// effectiveModel is the model with the cache write price of what the session actually
// wrote: the what-ifs write context that never was, and price it as the session did.
func effectiveModel(model cost.Model, turns []turn) cost.Model {
	var all, hour int
	for _, t := range turns {
		all, hour = all+t.write, hour+t.write1h
	}
	if all > 0 && hour > 0 {
		model.CacheWrite = (float64(all-hour)*model.CacheWrite + float64(hour)*oneHourWrite*model.Input) / float64(all)
	}
	return model
}

// turnCost prices one request at list prices; cache writes at the 5-minute
// price. Tokens are priced per million.
func turnCost(model cost.Model, input, read, write, output int) float64 {
	return (float64(input)*model.Input + float64(read)*model.CacheRead + float64(write)*model.CacheWrite + float64(output)*model.Output) / 1e6
}

// simulate prices the session as if it had been compacted whenever its context
// passed limit (0: never), with the same simple model for every limit, so the
// numbers compare with each other: a request pays cache reads for what was
// already cached, cache writes for what is new, and its output. A compaction
// is one request over the full context that writes the summary; the next
// request writes the new, short conversation. It returns the cost and how many
// compactions that took.
func simulate(turns []turn, limit, base int, model cost.Model) (usd float64, compactions int) {
	if len(turns) == 0 {
		return 0, 0
	}
	if base == 0 { // what every conversation starts with: system prompt, tools, instructions
		base = turns[0].ctx
		for _, t := range turns {
			base = min(base, t.ctx)
		}
	}
	if base+summaryTokens >= limit {
		limit = 0 // compacting could not make it smaller: nothing to simulate
	}
	cur, cached := 0, turns[0].read // a conversation cut at the start of the period begins with what was already cached
	prevOrig := 0
	for i, t := range turns {
		switch {
		case i == 0:
			cur = t.ctx
		case compacted(prevOrig, t.ctx):
			cur, cached = t.ctx, 0 // the real conversation started over here
		default:
			cur += max(t.ctx-prevOrig, 0)
		}
		prevOrig = t.ctx
		if limit > 0 && cur > limit {
			usd += (float64(cached)*model.CacheRead + float64(summaryTokens)*model.Output) / 1e6
			compactions++
			cur, cached = base+summaryTokens, 0
		}
		usd += turnCost(model, 0, cached, cur-cached, t.output)
		cached = cur
	}
	return usd, compactions
}
