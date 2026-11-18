package mcptool

import (
	"cmp"
	"fmt"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

// StatsReport is the JSON form of a stats query.
type StatsReport struct {
	By      string      `json:"by"`
	Session string      `json:"session,omitempty"`
	Groups  []StatsJSON `json:"groups"`
	Total   StatsJSON   `json:"total"`
}

type StatsJSON struct {
	Group     string     `json:"group"`
	Family    string     `json:"family,omitempty"`
	Command   string     `json:"command,omitempty"`
	Outcome   string     `json:"outcome,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Runs      int        `json:"runs"` // steps for family/optimizer groupings
	Optimized int        `json:"optimized"`
	Failed    int        `json:"failed"`
	RawTokens int        `json:"raw_tokens"`
	OutTokens int        `json:"out_tokens"`
	Saved     int        `json:"saved_tokens"`
	SavedPct  float64    `json:"saved_pct"`
	First     *time.Time `json:"first,omitempty"`
	Last      *time.Time `json:"last,omitempty"`
}

// Report builds the JSON form of stats, with totals.
func Report(query component.StatsQuery, stats []component.Stat) StatsReport {
	r := StatsReport{By: cmp.Or(query.By, component.ByRoute), Session: query.Session, Groups: []StatsJSON{}}
	total := component.Stat{Group: "total"}
	for _, s := range stats {
		j := statJSON(s)
		j.First, j.Last = &s.First, &s.Last
		r.Groups = append(r.Groups, j)
		total = add(total, s)
	}
	r.Total = statJSON(total)
	return r
}

func statJSON(stat component.Stat) StatsJSON {
	pct := 0.0
	if stat.RawTokens > 0 {
		pct = math.Round(1000*float64(stat.Saved())/float64(stat.RawTokens)) / 10
	}
	return StatsJSON{Group: stat.Group, Family: stat.Family, Command: stat.Command, Outcome: stat.Outcome, Reason: stat.Reason, Runs: stat.Runs, Optimized: stat.Optimized, Failed: stat.Failed,
		RawTokens: stat.RawTokens, OutTokens: stat.OutTokens, Saved: stat.Saved(), SavedPct: pct}
}

func add(total, stat component.Stat) component.Stat {
	total.Runs += stat.Runs
	total.Optimized += stat.Optimized
	total.Failed += stat.Failed
	total.RawTokens += stat.RawTokens
	total.OutTokens += stat.OutTokens
	return total
}

// StatsTable renders stats with a total row.
func StatsTable(by string, stats []component.Stat) string {
	by = cmp.Or(by, component.ByRoute)
	steps := by == component.ByFamily || by == component.ByOptimizer || by == component.ByDialect
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	switch {
	case steps:
		fmt.Fprintf(w, "%s\tsteps\traw tok\tout tok\tsaved\tsaved %%\n", by)
	case by == component.BySession:
		fmt.Fprintf(w, "%s\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %%\tfirst\tlast\n", by)
	default:
		fmt.Fprintf(w, "%s\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %%\n", by)
	}
	total := component.Stat{Group: "total"}
	row := func(stat component.Stat) {
		saved := fmt.Sprintf("%d\t%d\t%d\t%s", stat.RawTokens, stat.OutTokens, stat.Saved(), Percent(stat.Saved(), stat.RawTokens))
		switch {
		case steps:
			fmt.Fprintf(w, "%s\t%d\t%s\n", stat.Group, stat.Runs, saved)
		case by == component.BySession && stat.Group != "total":
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\t%s\n", stat.Group, stat.Runs, stat.Optimized, stat.Failed, saved,
				stat.First.Format(time.DateTime), stat.Last.Format(time.DateTime))
		default:
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n", stat.Group, stat.Runs, stat.Optimized, stat.Failed, saved)
		}
	}
	for _, s := range stats {
		row(s)
		total = add(total, s)
	}
	row(total)
	w.Flush()
	return b.String()
}

// Percent formats part/whole, "-" when whole is 0.
func Percent(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(part)/float64(whole))
}
