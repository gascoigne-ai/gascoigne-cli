package ledger

import (
	"cmp"
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

// The stats tests run against a real sqlite file. The reference model below
// recomputes every figure in plain Go from the same events, so a wrong SQL
// aggregate (grouping, filter, net cost of a rerequest) shows up as a diff.

// run is one recorded event as the reference model sees it.
type run struct {
	id                                      int
	ev                                      component.Event
	outcome                                 component.Outcome // changes to Rerequested
	steps                                   []component.Step
	session, agent, model, command, key, rt string
}

// model replays the ledger's rules for marking rerequested runs.
type model struct{ runs []*run }

func (m *model) record(ev component.Event) {
	if ev.Outcome == component.Bypassed && strings.HasPrefix(ev.Reason, "raw requested") {
		m.markLatest(ev.Key, &ev.Session)
	}
	m.runs = append(m.runs, &run{id: len(m.runs) + 1, ev: ev, outcome: ev.Outcome, steps: ev.Steps,
		session: ev.Session, agent: ev.Agent, model: ev.Model, command: ev.Command, key: ev.Key, rt: ev.Route})
}

func (m *model) fetched(key string) { m.markLatest(key, nil) }

func (m *model) markLatest(key string, session *string) {
	for _, r := range slices.Backward(m.runs) {
		if r.key == key && r.outcome == component.Optimized && (session == nil || r.session == *session) {
			r.outcome = component.Rerequested
			return
		}
	}
}

func (m *model) selected(q component.StatsQuery) []*run {
	var out []*run
	for _, r := range m.runs {
		switch {
		case q.Session != "" && r.session != q.Session,
			q.Agent != "" && r.agent != q.Agent,
			q.Model != "" && r.model != q.Model,
			!q.Since.IsZero() && r.ev.TS.UnixMilli() < q.Since.UnixMilli():
		default:
			out = append(out, r)
		}
	}
	return out
}

// week is sqlite's %W: weeks start on Monday, days before the first Monday are week 00.
func week(t time.Time) string {
	return fmt.Sprintf("%d-W%02d", t.Year(), (t.YearDay()-1+7-(int(t.Weekday())+6)%7)/7)
}

func (m *model) label(r *run, by string) string {
	t := r.ev.TS.Local()
	g := map[string]string{
		component.ByRoute: r.rt, component.BySession: r.session, component.ByAgent: r.agent,
		component.ByModel: r.model, component.ByCommand: r.command, component.ByOutcome: string(r.outcome),
		component.ByDay: t.Format("2006-01-02"), component.ByWeek: week(t), component.ByMonth: t.Format("2006-01"),
	}[by]
	return cmp.Or(g, "-")
}

// stats is Stats(q) computed in Go.
func (m *model) stats(q component.StatsQuery) []component.Stat {
	groups := map[string]*component.Stat{}
	at := func(g string) *component.Stat {
		if groups[g] == nil {
			groups[g] = &component.Stat{Group: g}
		}
		return groups[g]
	}
	touch := func(s *component.Stat, ts time.Time) {
		if s.First.IsZero() || ts.Before(s.First) {
			s.First = ts
		}
		if ts.After(s.Last) {
			s.Last = ts
		}
	}
	stepped := q.By == component.ByFamily || q.By == component.ByOptimizer || q.By == component.ByDialect
	for _, r := range m.selected(q) {
		ts := time.UnixMilli(r.ev.TS.UnixMilli())
		if stepped {
			for _, st := range r.steps {
				g := map[string]string{component.ByFamily: st.Family, component.ByOptimizer: st.Optimizer, component.ByDialect: st.Dialect}[q.By]
				s := at(cmp.Or(g, "-"))
				s.Family = max(s.Family, st.Family)
				s.Runs++
				s.Optimized++
				s.RawTokens += st.RawTokens
				s.OutTokens += st.OutTokens
				touch(s, ts)
			}
			continue
		}
		s := at(m.label(r, q.By))
		s.Runs++
		s.RawTokens += r.ev.RawTokens
		s.OutTokens += r.ev.OutTokens
		switch r.outcome {
		case component.Optimized:
			s.Optimized++
		case component.Failed:
			s.Failed++
		case component.Rerequested:
			s.OutTokens += r.ev.RawTokens // read twice: the result, then the original
		}
		touch(s, ts)
	}
	out := make([]component.Stat, 0, len(groups))
	for _, s := range groups {
		out = append(out, *s)
	}
	if q.Chronological() {
		slices.SortFunc(out, func(a, b component.Stat) int { return cmp.Compare(a.Group, b.Group) })
	} else if q.By == component.BySession { // latest first
		slices.SortFunc(out, func(a, b component.Stat) int {
			return cmp.Or(b.Last.Compare(a.Last), cmp.Compare(a.Group, b.Group))
		})
	} else {
		slices.SortFunc(out, func(a, b component.Stat) int {
			return cmp.Or(cmp.Compare(b.Saved(), a.Saved()), cmp.Compare(a.Group, b.Group))
		})
	}
	return out
}

func diffStats(t *testing.T, label string, got, want []component.Stat) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d groups, want %d\n got %v\nwant %v", label, len(got), len(want), groupsOf(got), groupsOf(want))
		return
	}
	for i := range got {
		g, w := got[i], want[i]
		g.First, g.Last = time.UnixMilli(g.First.UnixMilli()), time.UnixMilli(g.Last.UnixMilli())
		if g != w {
			t.Errorf("%s: group #%d\n got %+v\nwant %+v", label, i, g, w)
		}
	}
}

func groupsOf(s []component.Stat) []string {
	var out []string
	for _, x := range s {
		out = append(out, x.Group)
	}
	return out
}

// randomLedger fills a ledger and a model with the same mixed history.
func randomLedger(t *testing.T, seed uint64, n int) (*Ledger, *model) {
	t.Helper()
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db"), EventsTTL: 0})
	m := &model{}
	rng := rand.New(rand.NewPCG(seed, 7))
	pick := func(xs ...string) string { return xs[rng.IntN(len(xs))] }
	ts := time.Now().Add(-400 * 24 * time.Hour) // all runs in the past, as a fetch marks runs up to now
	for range n {
		ts = ts.Add(time.Duration(1+rng.IntN(30)) * time.Hour)
		raw := 1 + rng.IntN(5000)
		ev := component.Event{
			TS: ts, Session: pick("s1", "s2", "s3", ""), Agent: pick("a1", "a2", ""), Model: pick("m1", "m2", ""),
			Command: pick("c0", "c1", "c2", "c3"), Key: pick("k0", "k1", "k2", "k3", "k4"), Route: pick("r0", "r1", "r2", ""),
			RawTokens: raw, OutTokens: raw,
		}
		switch rng.IntN(10) {
		case 0, 1, 2, 3, 4:
			ev.Outcome, ev.OutTokens = component.Optimized, 1+rng.IntN(raw)
			for range 1 + rng.IntN(3) {
				o := rng.IntN(5)
				ev.Steps = append(ev.Steps, component.Step{Optimizer: fmt.Sprint("o", o), Family: fmt.Sprint("f", o%3),
					Dialect: pick("d0", "d1", "d2", ""), RawTokens: raw, OutTokens: ev.OutTokens})
			}
		case 5, 6:
			ev.Outcome = component.Unchanged
		case 7, 8:
			ev.Outcome, ev.Filter = component.Bypassed, "bypass"
			ev.Reason = pick("raw requested: env GASCOIGNE_RAW=1", "45 tokens < min_tokens 200")
		default:
			ev.Outcome, ev.Filter, ev.Reason = component.Failed, "o/err", "boom"
		}
		if err := l.Record(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
		m.record(ev)
		if rng.IntN(12) == 0 { // the agent fetched the original of a source
			key := pick("k0", "k1", "k2", "k3", "k4")
			if err := l.MarkFailure(t.Context(), key, component.ReasonRawRequested); err != nil {
				t.Fatal(err)
			}
			m.fetched(key)
		}
	}
	return l, m
}

var allGroupings = []string{
	component.ByRoute, component.BySession, component.ByAgent, component.ByModel, component.ByCommand,
	component.ByOutcome, component.ByDay, component.ByWeek, component.ByMonth,
	component.ByFamily, component.ByOptimizer, component.ByDialect,
}

// Every grouping, under every filter combination, equals the reference model.
func TestStatsMatchModel(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3} {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) {
			l, m := randomLedger(t, seed, 400)
			mid := m.runs[len(m.runs)/2].ev.TS
			filters := map[string]component.StatsQuery{
				"all":      {},
				"session":  {Session: "s1"},
				"agent":    {Agent: "a2"},
				"model":    {Model: "m1"},
				"since":    {Since: mid},
				"combined": {Session: "s2", Agent: "a1", Since: mid},
				"nothing":  {Session: "nobody"},
			}
			for fname, f := range filters {
				for _, by := range allGroupings {
					q := f
					q.By = by
					got, err := l.Stats(t.Context(), q)
					if err != nil {
						t.Fatalf("%s/%s: %v", fname, by, err)
					}
					diffStats(t, fname+"/"+by, got, m.stats(q))
				}
			}
		})
	}
}

// Totals add up across groupings: any event grouping sums to the same
// figures, and a rerequest is a loss, never a saving.
func TestStatsTotalsAgree(t *testing.T) {
	l, m := randomLedger(t, 9, 500)
	var want component.Stat
	for _, s := range m.stats(component.StatsQuery{By: component.ByOutcome}) {
		want.Runs += s.Runs
		want.RawTokens += s.RawTokens
		want.OutTokens += s.OutTokens
	}
	for _, by := range []string{component.ByRoute, component.BySession, component.ByAgent, component.ByModel,
		component.ByCommand, component.ByOutcome, component.ByDay, component.ByWeek, component.ByMonth} {
		got, err := l.Stats(t.Context(), component.StatsQuery{By: by})
		if err != nil {
			t.Fatal(err)
		}
		var sum component.Stat
		for _, s := range got {
			sum.Runs += s.Runs
			sum.RawTokens += s.RawTokens
			sum.OutTokens += s.OutTokens
		}
		if sum != want {
			t.Errorf("by %s: runs %d raw %d out %d, want %d %d %d", by, sum.Runs, sum.RawTokens, sum.OutTokens, want.Runs, want.RawTokens, want.OutTokens)
		}
	}
	got, _ := l.Stats(t.Context(), component.StatsQuery{By: component.ByOutcome})
	seen := map[string]component.Stat{}
	for _, s := range got {
		seen[s.Group] = s
	}
	if r, ok := seen["rerequested"]; !ok || r.Saved() >= 0 {
		t.Errorf("rerequested should exist and cost tokens: %+v", r)
	}
	for _, g := range []string{"unchanged", "bypassed", "error"} {
		if s, ok := seen[g]; ok && s.Saved() != 0 {
			t.Errorf("%s saved %d, want 0 (untouched)", g, s.Saved())
		}
	}
	if s := seen["optimized"]; s.Saved() <= 0 {
		t.Errorf("optimized saved %d, want > 0", s.Saved())
	}
}

// Reasons list runs that were not optimized, with the latest reason per group.
func TestReasonsMatchModel(t *testing.T) {
	l, m := randomLedger(t, 4, 400)
	type key struct{ group, command, outcome string }
	for _, q := range []component.StatsQuery{{}, {Session: "s1"}, {Agent: "a1", Model: "m2"}} {
		want := map[key]component.Stat{}
		for _, r := range m.selected(q) {
			if r.outcome == component.Optimized || r.outcome == component.Rerequested {
				continue
			}
			k := key{cmp.Or(r.ev.Filter, "-"), r.command, string(r.outcome)}
			s := want[k]
			s.Runs++
			s.RawTokens += r.ev.RawTokens
			s.OutTokens += r.ev.OutTokens
			s.Reason = r.ev.Reason // runs are in recording order: the last wins
			if r.outcome == component.Failed {
				s.Failed++
			}
			want[k] = s
		}
		q.By = component.ByReason
		got, err := l.Stats(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Fatalf("%+v: %d groups, want %d", q, len(got), len(want))
		}
		for _, g := range got {
			w := want[key{g.Group, g.Command, g.Outcome}]
			if g.Runs != w.Runs || g.RawTokens != w.RawTokens || g.OutTokens != w.OutTokens || g.Reason != w.Reason || g.Failed != w.Failed {
				t.Errorf("%+v: group %v\n got %+v\nwant %+v", q, g.Group, g, w)
			}
		}
	}
}

// Impact splits gross savings from give-back per day.
func TestImpactMatchModel(t *testing.T) {
	l, m := randomLedger(t, 5, 500)
	for _, q := range []component.StatsQuery{{}, {Agent: "a1"}, {Since: m.runs[len(m.runs)/3].ev.TS}} {
		want := map[string]*ImpactDay{}
		for _, r := range m.selected(q) {
			if r.outcome != component.Optimized && r.outcome != component.Rerequested {
				continue
			}
			day := r.ev.TS.Local().Format("2006-01-02")
			d := want[day]
			if d == nil {
				d = &ImpactDay{Day: day}
				want[day] = d
			}
			if r.outcome == component.Optimized {
				d.Optimized++
				d.Saved += r.ev.RawTokens - r.ev.OutTokens
			} else {
				d.Rerequested++
				d.Wasted += r.ev.OutTokens
			}
		}
		got, err := l.Impact(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) || !slices.IsSortedFunc(got, func(a, b ImpactDay) int { return cmp.Compare(a.Day, b.Day) }) {
			t.Fatalf("%+v: %d days (sorted: %v), want %d", q, len(got), got, len(want))
		}
		for _, g := range got {
			if w := want[g.Day]; w == nil || g != *w {
				t.Errorf("%+v: day %s got %+v want %+v", q, g.Day, g, w)
			}
		}
	}
}

// Hand-computed figures for a small, readable history.
func TestStatsHandComputed(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ctx := t.Context()
	at := func(h int) time.Time {
		return time.Date(2026, 3, 2, 12, 0, 0, 0, time.Local).Add(time.Duration(h) * 24 * time.Hour)
	}
	for _, ev := range []component.Event{
		{TS: at(0), Session: "s", Key: "a", Command: "go test", Outcome: component.Optimized, RawTokens: 1000, OutTokens: 100,
			Steps: []component.Step{{Optimizer: "gotest", Family: "test", RawTokens: 1000, OutTokens: 100}}},
		{TS: at(1), Session: "s", Key: "b", Command: "git log", Outcome: component.Optimized, RawTokens: 600, OutTokens: 200,
			Steps: []component.Step{{Optimizer: "gitlog", Family: "vcs", RawTokens: 600, OutTokens: 200}}},
		{TS: at(2), Session: "s", Key: "c", Command: "ls", Outcome: component.Unchanged, RawTokens: 50, OutTokens: 50},
		{TS: at(3), Session: "s", Key: "d", Command: "cat", Outcome: component.Bypassed, Filter: "minsize", Reason: "10 tokens < min_tokens 200", RawTokens: 10, OutTokens: 10},
	} {
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	// The agent read "b" optimized, then asked for the original: 200 + 600 tokens for a 600 baseline.
	if err := l.MarkFailure(ctx, "b", component.ReasonRawRequested); err != nil {
		t.Fatal(err)
	}
	by := func(g string) map[string]component.Stat {
		t.Helper()
		got, err := l.Stats(ctx, component.StatsQuery{By: g})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]component.Stat{}
		for _, s := range got {
			out[s.Group] = s
		}
		return out
	}
	o := by(component.ByOutcome)
	for g, want := range map[string][3]int{ // runs, raw, out
		"optimized": {1, 1000, 100}, "rerequested": {1, 600, 800}, "unchanged": {1, 50, 50}, "bypassed": {1, 10, 10},
	} {
		if s := o[g]; [3]int{s.Runs, s.RawTokens, s.OutTokens} != want {
			t.Errorf("outcome %s = %+v, want runs/raw/out %v", g, s, want)
		}
	}
	if s := o["rerequested"]; s.Saved() != -200 {
		t.Errorf("rerequested saved %d, want -200", s.Saved())
	}
	days := by(component.ByDay)
	if len(days) != 4 || days["2026-03-03"].Saved() != -200 || days["2026-03-02"].Saved() != 900 {
		t.Errorf("days %+v", days)
	}
	// Per optimizer, steps are the optimizer's own work and stay gross.
	if s := by(component.ByOptimizer)["gitlog"]; s.Saved() != 400 || s.Runs != 1 {
		t.Errorf("gitlog %+v", s)
	}
	imp, _ := l.Impact(ctx, component.StatsQuery{})
	want := []ImpactDay{{"2026-03-02", 1, 900, 0, 0}, {"2026-03-03", 0, 0, 1, 200}}
	if !slices.Equal(imp, want) {
		t.Errorf("impact %+v, want %+v", imp, want)
	}
}

// Filters narrow every view the same way.
func TestStatsFilters(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	now := time.Now()
	for i, ev := range []component.Event{
		{Session: "s1", Agent: "claude", Model: "opus", Command: "x"},
		{Session: "s1", Agent: "codex", Model: "gpt", Command: "x"},
		{Session: "s2", Agent: "claude", Model: "opus", Command: "y"},
		{Session: "s2", Agent: "claude", Model: "sonnet", Command: "y"},
	} {
		ev.TS, ev.Key, ev.Outcome, ev.RawTokens, ev.OutTokens = now.Add(-time.Duration(4-i)*48*time.Hour), "k", component.Unchanged, 10, 10
		if err := l.Record(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
	}
	runs := func(q component.StatsQuery) int {
		got, err := l.Stats(t.Context(), q)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, s := range got {
			n += s.Runs
		}
		return n
	}
	for name, tt := range map[string]struct {
		q    component.StatsQuery
		want int
	}{
		"none":         {component.StatsQuery{}, 4},
		"session":      {component.StatsQuery{Session: "s1"}, 2},
		"agent":        {component.StatsQuery{Agent: "claude"}, 3},
		"model":        {component.StatsQuery{Model: "opus"}, 2},
		"since":        {component.StatsQuery{Since: now.Add(-5 * 24 * time.Hour)}, 2},
		"since future": {component.StatsQuery{Since: now.Add(time.Hour)}, 0},
		"all three":    {component.StatsQuery{Session: "s2", Agent: "claude", Model: "opus"}, 1},
		"no match":     {component.StatsQuery{Session: "s1", Model: "sonnet"}, 0},
	} {
		if got := runs(tt.q); got != tt.want {
			t.Errorf("%s: %d runs, want %d", name, got, tt.want)
		}
	}
	if _, err := l.Stats(t.Context(), component.StatsQuery{By: "nonsense"}); err == nil {
		t.Error("unknown grouping accepted")
	}
}

// Sessions keep to themselves: a raw rerun marks only its own session's run.
func TestRerequestIsPerSession(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	ts := time.Now().Add(-time.Hour)
	rec := func(session string, o component.Outcome, reason string) {
		t.Helper()
		ts = ts.Add(time.Second)
		ev := component.Event{TS: ts, Session: session, Key: "k", Outcome: o, Reason: reason, RawTokens: 100, OutTokens: 10}
		if err := l.Record(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
	}
	rec("a", component.Optimized, "")
	rec("b", component.Optimized, "")
	rec("b", component.Bypassed, "raw requested: attr raw=true")
	for session, want := range map[string]string{"a": "optimized", "b": "rerequested"} {
		got, _ := l.Stats(t.Context(), component.StatsQuery{By: component.ByOutcome, Session: session})
		if !slices.ContainsFunc(got, func(s component.Stat) bool { return s.Group == want }) {
			t.Errorf("session %s: %v, want %s", session, groupsOf(got), want)
		}
	}
}

// Only the run the agent saw last is marked, once.
func TestFetchMarksLatestRunOnce(t *testing.T) {
	l := start(t, &Config{Path: filepath.Join(t.TempDir(), "l.db")})
	for i := range 3 {
		ev := component.Event{TS: time.Now().Add(-time.Duration(3-i) * time.Hour), Key: "k", Outcome: component.Optimized, RawTokens: 100, OutTokens: 10}
		if err := l.Record(t.Context(), ev); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 { // fetched twice: two runs of the source are marked, newest first
		if err := l.MarkFailure(t.Context(), "k", component.ReasonRawRequested); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := l.Stats(t.Context(), component.StatsQuery{By: component.ByOutcome})
	counts := map[string]int{}
	for _, s := range got {
		counts[s.Group] = s.Runs
	}
	if counts["rerequested"] != 2 || counts["optimized"] != 1 {
		t.Errorf("outcomes %v, want 2 rerequested and 1 optimized", counts)
	}
	// A manual failure is not a request for the original.
	if err := l.MarkFailure(t.Context(), "k", component.ReasonManual); err != nil {
		t.Fatal(err)
	}
	got, _ = l.Stats(t.Context(), component.StatsQuery{By: component.ByOutcome})
	for _, s := range got {
		if s.Group == "optimized" && s.Runs != 1 {
			t.Errorf("manual failure changed outcomes: %+v", got)
		}
	}
}

// Reset removes exactly the filtered runs and their steps.
func TestResetMatchesModel(t *testing.T) {
	l, m := randomLedger(t, 6, 300)
	q := component.StatsQuery{Session: "s1", Agent: "a1"}
	want := len(m.selected(q))
	n, err := l.Reset(t.Context(), q, false)
	if err != nil || int(n) != want {
		t.Fatalf("reset %d, %v; want %d", n, err, want)
	}
	m.runs = slices.DeleteFunc(m.runs, func(r *run) bool { return r.session == "s1" && r.agent == "a1" })
	for _, by := range []string{component.ByOutcome, component.ByOptimizer} {
		got, _ := l.Stats(t.Context(), component.StatsQuery{By: by})
		diffStats(t, "after reset/"+by, got, m.stats(component.StatsQuery{By: by}))
	}
	var orphans int64
	l.db.Table("steps").Where("event_id NOT IN (SELECT id FROM events)").Count(&orphans)
	if orphans != 0 {
		t.Errorf("%d steps left without an event", orphans)
	}
}

// Rows read back through the feed carry what was recorded.
func TestPageMatchesModel(t *testing.T) {
	l, m := randomLedger(t, 8, 200)
	got, err := l.Page(t.Context(), component.StatsQuery{Agent: "a1"}, Page{Limit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	want := m.selected(component.StatsQuery{Agent: "a1"})
	if len(got) != len(want) {
		t.Fatalf("%d runs, want %d", len(got), len(want))
	}
	for i, e := range got { // newest first
		r := want[len(want)-1-i]
		if e.Key != r.key || e.Outcome != r.outcome || e.RawTokens != r.ev.RawTokens || e.OutTokens != r.ev.OutTokens {
			t.Fatalf("run %d: got %s/%s %d→%d, want %s/%s %d→%d", i, e.Key, e.Outcome, e.RawTokens, e.OutTokens, r.key, r.outcome, r.ev.RawTokens, r.ev.OutTokens)
		}
	}
}

// Pruning old runs keeps their totals: every grouping the rollup can answer
// equals the model of the full history, before and after a restart; session
// groups only know the runs still stored.
func TestRollupKeepsTotals(t *testing.T) {
	for _, seed := range []uint64{1, 2} {
		t.Run(fmt.Sprint("seed", seed), func(t *testing.T) {
			l, m := randomLedger(t, seed, 400)
			cutoff := m.runs[len(m.runs)/2].ev.TS
			now := time.Now()
			l.now = func() time.Time { return now }
			l.cfg.EventsTTL = now.Sub(cutoff)
			for range 2 { // a second prune has nothing left to add
				if err := l.pruneEvents(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
			day := time.Date(cutoff.Year(), cutoff.Month(), cutoff.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
			filters := map[string]component.StatsQuery{
				"all": {}, "agent": {Agent: "a2"}, "model": {Model: "m1"}, "since": {Since: day},
			}
			for fname, f := range filters {
				for _, by := range allGroupings {
					if by == component.BySession {
						continue
					}
					q := f
					q.By = by
					got, err := l.Stats(t.Context(), q)
					if err != nil {
						t.Fatalf("%s/%s: %v", fname, by, err)
					}
					diffStats(t, fname+"/"+by, got, m.stats(q))
				}
			}
			live := &model{}
			for _, r := range m.runs {
				if !r.ev.TS.Before(cutoff) {
					live.runs = append(live.runs, r)
				}
			}
			q := component.StatsQuery{By: component.BySession}
			got, err := l.Stats(t.Context(), q)
			if err != nil {
				t.Fatal(err)
			}
			diffStats(t, "session", got, live.stats(q))
		})
	}
}

func TestRollupTTL(t *testing.T) {
	l, m := randomLedger(t, 3, 100) // runs 400 to ~340 days ago
	now := time.Now()
	l.now = func() time.Time { return now }
	l.cfg.EventsTTL = time.Hour
	if err := l.pruneEvents(t.Context()); err != nil {
		t.Fatal(err)
	}
	runs := func() (n int) {
		got, err := l.Stats(t.Context(), component.StatsQuery{By: component.ByOutcome})
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range got {
			n += s.Runs
		}
		return n
	}
	if got := runs(); got != len(m.runs) {
		t.Fatalf("runs after events prune = %d, want %d", got, len(m.runs))
	}
	l.cfg.RollupTTL = 370 * 24 * time.Hour
	if err := l.pruneRollup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := runs(); got == 0 || got >= len(m.runs) {
		t.Fatalf("runs after rollup_ttl 370d = %d of %d, want some", got, len(m.runs))
	}
	l.cfg.RollupTTL = time.Hour
	if err := l.pruneRollup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := runs(); got != 0 {
		t.Fatalf("runs left = %d", got)
	}
}
