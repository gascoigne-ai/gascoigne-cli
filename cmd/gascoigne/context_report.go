package main

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// buildContext assembles the report from sessions already read.
func buildContext(sessions []sessionCost, period time.Duration) contextReport {
	builder := newContextBuilder(sessions, period)
	for i := range builder.report.Sessions {
		builder.addSession(&builder.report.Sessions[i])
	}
	return builder.finish()
}

// contextBuilder sums the sessions into the report, one section at a time.
type contextBuilder struct {
	report     contextReport
	byCause    map[string]*cacheRewrite
	bySource   map[string]*injectedSource
	sessionsOf map[string]map[string]bool // injected source -> ids of the sessions it came in
	whatIfs    []whatIf
	byDay      map[string]*contextDay
}

func newContextBuilder(sessions []sessionCost, period time.Duration) *contextBuilder {
	report := contextReport{Billing: cost.BillingOf("claude").Mode, Hours: int(period.Hours()), Sessions: sessions, Buckets: []contextBucket{
		{Label: "under 100k"}, {Label: "100k to 200k"}, {Label: "200k to 400k"}, {Label: "over 400k"},
	}}
	if report.Sessions == nil {
		report.Sessions = []sessionCost{}
	}
	slices.SortFunc(report.Sessions, func(left, right sessionCost) int { return right.End.Compare(left.End) })
	whatIfs := make([]whatIf, len(thresholds))
	for i, threshold := range thresholds {
		whatIfs[i].Threshold = threshold
	}
	return &contextBuilder{
		report:     report,
		byCause:    map[string]*cacheRewrite{},
		bySource:   map[string]*injectedSource{},
		sessionsOf: map[string]map[string]bool{},
		whatIfs:    whatIfs,
		byDay:      map[string]*contextDay{},
	}
}

// addSession counts one session into every section; an unpriced one only counts its requests.
func (b *contextBuilder) addSession(session *sessionCost) {
	session.SubagentUSD, session.SubagentTurns = session.subUSD, session.subTurns
	b.report.Turns += session.Turns
	if !session.Priced {
		b.report.Unpriced++
		return
	}
	b.report.CostUSD += session.CostUSD
	exact, _ := priceOf(session.Model)
	model := effectiveModel(exact, session.turns)
	b.addWhatIfs(session, model)
	b.addInjections(session, model)
	b.report.Compactions.add(session.turns, model)
	b.report.CacheTTL.add(session.turns, exact)
	b.addRewrites(session, model)
	b.addTurns(session, exact)
}

// addWhatIfs prices the session compacted at every threshold, against the same model without compacting.
func (b *contextBuilder) addWhatIfs(session *sessionCost, model cost.Model) {
	base, _ := simulate(session.turns, 0, session.floor, model)
	b.report.ModelUSD += base
	for i, threshold := range thresholds {
		usd, compactions := simulate(session.turns, threshold, session.floor, model)
		b.whatIfs[i].CostUSD += usd
		b.whatIfs[i].SavedUSD += base - usd
		b.whatIfs[i].Compactions += compactions
		if compactions > 0 {
			b.whatIfs[i].Sessions++
		}
	}
}

// addInjections counts the text added to the session: written once, then read by every later request.
func (b *contextBuilder) addInjections(session *sessionCost, model cost.Model) {
	for _, injected := range session.inj {
		source := b.bySource[injected.label]
		if source == nil {
			source = &injectedSource{Source: injected.label}
			b.bySource[injected.label] = source
			b.sessionsOf[injected.label] = map[string]bool{}
		}
		session.InjectedTokens += injected.tokens
		later := max(session.Turns-injected.before, 0)
		source.Times++
		source.Tokens += injected.tokens
		source.Rereads += injected.tokens * later
		source.CostUSD += float64(injected.tokens) * (model.CacheWrite + float64(later)*model.CacheRead) / 1e6
		b.sessionsOf[injected.label][session.ID] = true
	}
}

// addRewrites counts the cache the session wrote again, by cause.
func (b *contextBuilder) addRewrites(session *sessionCost, model cost.Model) {
	for _, rewrite := range rewrites(session.turns) {
		extra := float64(rewrite.tokens) * (model.CacheWrite - model.CacheRead) / 1e6
		entry := rewriteOf(b.byCause, rewrite.cause)
		entry.Turns++
		entry.Tokens += rewrite.tokens
		entry.CostUSD += extra
		session.RewriteUSD += extra
	}
}

// addTurns splits the session's cost by the size of the context and by local day.
func (b *contextBuilder) addTurns(session *sessionCost, model cost.Model) {
	for _, request := range session.turns {
		bucket := &b.report.Buckets[bucketOf(request.ctx)]
		bucket.Turns++
		bucket.CostUSD += request.cost(model)
		if request.ts.IsZero() {
			continue
		}
		key := request.ts.Local().Format(time.DateOnly)
		day := b.byDay[key]
		if day == nil {
			day = &contextDay{Day: key}
			b.byDay[key] = day
		}
		day.Turns++
		day.CostUSD += request.cost(model)
	}
}

// finish fills what follows from the sums and returns the report.
func (b *contextBuilder) finish() contextReport {
	report := &b.report
	report.Compactions.done()
	report.CacheTTL.done()
	report.Days = fillContextDays(b.byDay)
	b.finishBuckets()
	b.finishInjected()
	b.finishRewrites()
	b.finishWhatIfs()
	report.Assumptions = contextAssumptions()
	return *report
}

func (b *contextBuilder) finishBuckets() {
	for i := range b.report.Buckets {
		if b.report.CostUSD > 0 {
			b.report.Buckets[i].Share = b.report.Buckets[i].CostUSD / b.report.CostUSD
		}
	}
}

func (b *contextBuilder) finishInjected() {
	for label, source := range b.bySource {
		source.Sessions = len(b.sessionsOf[label])
		source.PerTime = source.Tokens / max(source.Times, 1)
		if b.report.CostUSD > 0 {
			source.Share = source.CostUSD / b.report.CostUSD
		}
		b.report.Injected = append(b.report.Injected, *source)
	}
	slices.SortFunc(b.report.Injected, func(left, right injectedSource) int {
		return cmp.Or(cmp.Compare(right.CostUSD, left.CostUSD), cmp.Compare(left.Source, right.Source))
	})
	if b.report.Injected == nil {
		b.report.Injected = []injectedSource{}
	}
}

func (b *contextBuilder) finishRewrites() {
	for _, cause := range []string{causeFive, causeHour, causeChanged, causeCompaction} {
		rewriteOf(b.byCause, cause) // the usual causes always show, even at zero
	}
	for _, entry := range b.byCause {
		if b.report.CostUSD > 0 {
			entry.Share = entry.CostUSD / b.report.CostUSD
		}
		b.report.Rewrites = append(b.report.Rewrites, *entry)
	}
	slices.SortFunc(b.report.Rewrites, func(left, right cacheRewrite) int {
		return cmp.Or(cmp.Compare(right.CostUSD, left.CostUSD), cmp.Compare(left.Cause, right.Cause))
	})
}

func (b *contextBuilder) finishWhatIfs() {
	for i := range b.whatIfs {
		if b.report.ModelUSD > 0 {
			b.whatIfs[i].SavedPct = 100 * b.whatIfs[i].SavedUSD / b.report.ModelUSD
		}
	}
	b.report.WhatIf = b.whatIfs
	b.report.BestWindow = bestWindow(b.whatIfs)
}

// contextAssumptions are the caveats printed under every report.
func contextAssumptions() []string {
	return []string{
		"Dollars are what the tokens would cost at API list prices, cache writes at the 5-minute or 1-hour rate the transcript records; a subscription does not bill per token.",
		fmt.Sprintf("A compaction is assumed to cost one request over the whole context plus a %dk-token summary; later turns grow as they did.", summaryTokens/1000),
		"Compacting saves tokens, not detail: the summary forgets what it leaves out. Treat the saving as an upper bound on what is worth giving up.",
		"Sessions are main conversations; what subagents cost is added up apart, since they run in conversations of their own.",
		"Added text is priced as written once to the cache and read by every later request of the session; what a hook printed is named by the command it ran, as the transcript records it.",
		"A cache rewrite is context that the previous request carried but this one did not read from the cache; the cost shown is the extra over a read. The causes are read from the pauses between requests, so \"changed within the cache lifetime\" means the cached start of the conversation changed while it was warm (an edited instruction file, a changed tool set, another model); a transcript cannot say which.",
	}
}

// fillContextDays lists the days from the first to the last with a request,
// the quiet ones at zero, so a chart shows the gaps.
func fillContextDays(by map[string]*contextDay) []contextDay {
	out := []contextDay{}
	if len(by) == 0 {
		return out
	}
	keys := slices.Sorted(maps.Keys(by))
	first, _ := time.ParseInLocation(time.DateOnly, keys[0], time.Local)
	last, _ := time.ParseInLocation(time.DateOnly, keys[len(keys)-1], time.Local)
	for _, k := range mcptool.Days(first, last) {
		if d := by[k]; d != nil {
			out = append(out, *d)
		} else {
			out = append(out, contextDay{Day: k})
		}
	}
	return out
}

func rewriteOf(causes map[string]*cacheRewrite, cause string) *cacheRewrite {
	if causes[cause] == nil {
		causes[cause] = &cacheRewrite{Cause: cause}
	}
	return causes[cause]
}

func bucketOf(ctx int) int {
	switch {
	case ctx < 100_000:
		return 0
	case ctx < 200_000:
		return 1
	case ctx < 400_000:
		return 2
	}
	return 3
}

// bestWindow is the threshold of the what-if that saved most, 0 when none did.
func bestWindow(wi []whatIf) int {
	best, saved := 0, 0.0
	for _, x := range wi {
		if x.SavedUSD > saved {
			best, saved = x.Threshold, x.SavedUSD
		}
	}
	return best
}
