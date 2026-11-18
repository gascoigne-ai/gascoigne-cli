package mcptool

import (
	"cmp"
	"context"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// Overview answers "where do savings come from": totals, per command, per
// optimizer and why runs were skipped.
type Overview struct {
	Session    string               `json:"session,omitempty"`
	Filter     component.StatsQuery `json:"-"`
	Total      StatsJSON            `json:"total"`
	Outcomes   []StatsJSON          `json:"outcomes"`
	Commands   []StatsJSON          `json:"commands"`
	Optimizers []StatsJSON          `json:"optimizers"`
	Dialects   []StatsJSON          `json:"dialects"`
	Agents     []StatsJSON          `json:"agents"`
	Models     []StatsJSON          `json:"models"`
	Days       []StatsJSON          `json:"days"` // last 7 days (or since filter.Since), oldest first; weeks or months past 32 days
	Cost       Costs                `json:"cost"`
	// Runs not optimized, per stopping component (filter or optimizer) and
	// command, with the latest recorded reason.
	Skipped   []StatsJSON `json:"skipped"`
	Failed    []StatsJSON `json:"failed"`
	Unchanged []StatsJSON `json:"unchanged"`
	// Estimates says which numbers are estimated and how, so totals are
	// not read as billing.
	Estimates []string `json:"estimates"`
}

// BuildOverview gathers the overview that the stats tool shows for what filter selects.
func BuildOverview(ctx context.Context, ledger component.Ledger, filter component.StatsQuery) (Overview, error) {
	o := Overview{Session: filter.Session, Filter: filter}
	if err := o.fillGroups(ctx, ledger, filter); err != nil {
		return o, err
	}
	costs, err := BuildCosts(ctx, ledger, filter, cost.AgentContext)
	if err != nil {
		return o, err
	}
	o.Cost = costs
	if err := o.fillSkipped(ctx, ledger, filter); err != nil {
		return o, err
	}
	o.Estimates = estimates(o)
	return o, nil
}

// fillGroups reads one grouped report per overview section; the command one gives the total.
func (o *Overview) fillGroups(ctx context.Context, ledger component.Ledger, filter component.StatsQuery) error {
	for _, part := range []struct {
		by  string
		out *[]StatsJSON
	}{
		{component.ByOutcome, &o.Outcomes},
		{component.ByCommand, &o.Commands},
		{component.ByOptimizer, &o.Optimizers},
		{component.ByDialect, &o.Dialects},
		{component.ByAgent, &o.Agents},
		{component.ByModel, &o.Models},
		{component.ByDay, &o.Days},
	} {
		q := filter
		q.By = part.by
		if part.by == component.ByDay && q.Since.IsZero() {
			y, m, d := time.Now().AddDate(0, 0, -6).Date()
			q.Since = time.Date(y, m, d, 0, 0, 0, 0, time.Local) // local midnight, 7 days incl. today
		}
		if part.by == component.ByDay {
			q.By = chartBucket(time.Since(q.Since))
		}
		stats, err := ledger.Stats(ctx, q)
		if err != nil {
			return err
		}
		r := Report(component.StatsQuery{By: q.By}, stats)
		*part.out = r.Groups
		if part.by == component.ByDay && q.By == component.ByDay {
			o.Days = fillDays(r.Groups, q.Since, time.Now())
		}
		if part.by == component.ByCommand {
			o.Total = r.Total
		}
	}
	return nil
}

// fillSkipped sorts the runs not optimized into skipped, failed and unchanged.
func (o *Overview) fillSkipped(ctx context.Context, ledger component.Ledger, filter component.StatsQuery) error {
	rq := filter
	rq.By = component.ByReason
	reasons, err := ledger.Stats(ctx, rq)
	if err != nil {
		return err
	}
	o.Skipped, o.Failed, o.Unchanged = []StatsJSON{}, []StatsJSON{}, []StatsJSON{}
	for _, s := range reasons {
		j := statJSON(s)
		if j.Reason == "" {
			typ, _, _ := strings.Cut(s.Group, "/")
			j.Reason = SkipReason(typ) // recorded before reasons were
		}
		switch component.Outcome(s.Outcome) {
		case component.Bypassed:
			o.Skipped = append(o.Skipped, j)
		case component.Failed:
			o.Failed = append(o.Failed, j)
		default:
			o.Unchanged = append(o.Unchanged, j)
		}
	}
	return nil
}

// estimates explains how the overview's numbers are made: what is counted,
// what is modeled, and what the counts leave out.
func estimates(overview Overview) []string {
	notes := []string{
		"tokens: estimated from the text (tokens.Estimate, ~10% mean error against a 200k-vocabulary tokenizer), not billed counts",
	}
	if len(overview.Cost.Savings) > 0 {
		notes = append(notes,
			"cost: saved tokens × the model's tokenizer factor × the providers' official list prices (checked "+cost.PricesChecked+"; no discounts or plan terms); in context assumes each result is reread "+
				strconv.FormatFloat(overview.Cost.Rereads, 'f', 0, 64)+" times from the prompt cache (measured median), 0.8% of reads missing it")
	}
	for _, r := range overview.Optimizers {
		switch r.Group {
		case "image":
			notes = append(notes, "image: proxied screenshots counted as pixels/750 tokens (Anthropic's image formula)")
		case "tools":
			notes = append(notes, "tools: tool definitions counted once per distinct set, though every request sends them: real savings are larger")
		}
	}
	return notes
}

// outcomeNotes say what each outcome means for tokens.
var outcomeNotes = map[component.Outcome]string{
	component.Optimized:   "shrunk; original retrievable",
	component.Unchanged:   "no optimizer applied",
	component.Bypassed:    "passed through by a filter (see skipped)",
	component.Rerequested: "shrunk, then the agent asked for the original",
	component.Failed:      "optimizer error, original returned",
}

// skipReasons describe filter types, for runs recorded before reasons were.
var skipReasons = map[string]string{
	"minsize": "output below min_tokens",
	"bypass":  "raw output requested",
	"history": "source optimized badly before",
}

// SkipReason explains why a filter type bypasses optimization.
func SkipReason(filterType string) string {
	return cmp.Or(skipReasons[filterType], "-")
}

// Table renders the overview as plain text.
func (o Overview) Table() string {
	var b strings.Builder
	if o.Session != "" {
		fmt.Fprintf(&b, "session %s\n", o.Session)
	}
	t := o.Total
	fmt.Fprintf(&b, "%d runs · %d optimized · %d skipped · %d failed · %d → %d tokens · saved %d (%s)\n",
		t.Runs, t.Optimized, sumRuns(o.Skipped), t.Failed, t.RawTokens, t.OutTokens, t.Saved, Percent(t.Saved, t.RawTokens))
	if c := o.Cost; len(c.Savings) > 0 {
		fmt.Fprintf(&b, "worth $%.2f at first read, $%.2f while in context (%.0f rereads)\n", c.FirstRead, c.InContext, c.Rereads)
	}

	section := func(title, header string, rows []StatsJSON, row func(*tabwriter.Writer, StatsJSON)) {
		if len(rows) == 0 {
			return
		}
		fmt.Fprintf(&b, "\n%s\n", title)
		w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, header)
		for _, r := range rows {
			row(w, r)
		}
		w.Flush()
	}
	section("by outcome", "outcome\truns\traw tok\tout tok\tsaved\tshare of input\t", o.Outcomes, func(writer *tabwriter.Writer, stat StatsJSON) {
		fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%d\t%s\t%s\n", stat.Group, stat.Runs, stat.RawTokens, stat.OutTokens, stat.Saved, Percent(stat.RawTokens, t.RawTokens), outcomeNotes[component.Outcome(stat.Group)])
	})
	section("by command", "command\truns\toptimized\traw tok\tout tok\tsaved\tsaved %", o.Commands, func(writer *tabwriter.Writer, stat StatsJSON) {
		fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", stat.Group, stat.Runs, stat.Optimized, stat.RawTokens, stat.OutTokens, stat.Saved, Percent(stat.Saved, stat.RawTokens))
	})
	section("by agent", "agent\truns\toptimized\traw tok\tout tok\tsaved\tsaved %", Named(o.Agents), func(writer *tabwriter.Writer, stat StatsJSON) {
		fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", stat.Group, stat.Runs, stat.Optimized, stat.RawTokens, stat.OutTokens, stat.Saved, Percent(stat.Saved, stat.RawTokens))
	})
	section("by model", "model\truns\toptimized\traw tok\tout tok\tsaved\tsaved %", Named(o.Models), func(writer *tabwriter.Writer, stat StatsJSON) {
		fmt.Fprintf(writer, "%s\t%d\t%d\t%d\t%d\t%d\t%s\n", stat.Group, stat.Runs, stat.Optimized, stat.RawTokens, stat.OutTokens, stat.Saved, Percent(stat.Saved, stat.RawTokens))
	})
	section("by optimizer", "optimizer\tfamily\tsteps\traw tok\tout tok\tsaved\tsaved %", o.Optimizers, func(writer *tabwriter.Writer, stat StatsJSON) {
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%d\t%d\t%s\n", stat.Group, cmp.Or(stat.Family, "-"), stat.Runs, stat.RawTokens, stat.OutTokens, stat.Saved, Percent(stat.Saved, stat.RawTokens))
	})
	reasons := func(writer *tabwriter.Writer, stat StatsJSON) {
		fmt.Fprintf(writer, "%s\t%s\t%d\t%d\t%s\n", stat.Group, cmp.Or(stat.Command, "-"), stat.Runs, stat.RawTokens, stat.Reason)
	}
	section("skipped", "filter\tcommand\truns\ttokens\treason (latest)", o.Skipped, reasons)
	section("failed", "optimizer\tcommand\truns\ttokens\terror (latest)", o.Failed, reasons)
	section("unchanged", "-\tcommand\truns\ttokens\treason (latest)", o.Unchanged, reasons)
	if len(o.Estimates) > 0 {
		b.WriteString("\nestimates\n")
		for _, n := range o.Estimates {
			b.WriteString("- " + n + "\n")
		}
	}
	return b.String()
}

func sumRuns(rows []StatsJSON) int {
	n := 0
	for _, r := range rows {
		n += r.Runs
	}
	return n
}

// Named returns rows unless the only group is "-" (dimension never set).
func Named(rows []StatsJSON) []StatsJSON {
	if len(rows) == 1 && rows[0].Group == "-" {
		return nil
	}
	return rows
}

// chartBuckets say how coarse the trend chart gets as the period grows, so
// its bars stay readable: longest span first-fit, ordered shortest to longest.
var chartBuckets = []struct {
	upTo time.Duration
	by   string
}{
	{32 * 24 * time.Hour, component.ByDay},
	{190 * 24 * time.Hour, component.ByWeek},
	{1<<63 - 1, component.ByMonth},
}

// chartBucket is the grouping for a trend chart over the last span.
func chartBucket(span time.Duration) string {
	for _, b := range chartBuckets {
		if span <= b.upTo {
			return b.by
		}
	}
	return component.ByMonth
}

// fillDays adds empty days between from and to, so trends show quiet days.
func fillDays(days []StatsJSON, from, to time.Time) []StatsJSON {
	have := map[string]StatsJSON{}
	for _, d := range days {
		have[d.Group] = d
	}
	out := []StatsJSON{}
	for _, key := range Days(from, to) {
		if d, ok := have[key]; ok {
			out = append(out, d)
		} else {
			out = append(out, StatsJSON{Group: key})
		}
	}
	return out
}

// Days are the local calendar days (YYYY-MM-DD) from the one holding from to
// the one holding to. Stepping 24h from from itself would stop a day short
// of to when the period is not a whole number of days.
func Days(from, to time.Time) []string {
	var out []string
	y, m, d := from.Date()
	for day := time.Date(y, m, d, 0, 0, 0, 0, from.Location()); !day.After(to); day = day.AddDate(0, 0, 1) {
		out = append(out, day.Format(time.DateOnly))
	}
	return out
}
