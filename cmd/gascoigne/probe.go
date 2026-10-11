package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress/probe"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
)

// probeRun is one optimized original checked against its stored output.
type probeRun struct {
	RawID   string       `json:"raw_id"`
	Command string       `json:"command"`
	Family  string       `json:"family"`
	Report  probe.Report `json:"report"`
}

// cmdProbe checks recorded runs for facts their optimized output no longer
// shows: the retention side of what eval measures on fixtures, on real use.
func cmdProbe(ctx context.Context, args []string) error {
	var (
		since  string
		limit  int
		worst  int
		asJSON bool
	)
	return withLedger(ctx, "probe", args, 0, func(fs *flag.FlagSet) {
		fs.StringVar(&since, "since", "", "only the last `period`: 24h, 7d, 4w, 6mo")
		fs.IntVar(&limit, "limit", 2000, "newest optimized runs to check (`n`)")
		fs.IntVar(&worst, "worst", 10, "list the `n` runs that lost the most error lines")
		fs.BoolVar(&asJSON, "json", false, "print JSON instead of a table")
	}, func(l *ledger.Ledger, c *common, _ []string) error {
		id := c.identity()
		q := component.StatsQuery{Session: id.Session, Agent: id.Agent, Model: id.Model}
		if since != "" {
			d, err := parseSince(since)
			if err != nil {
				return err
			}
			q.Since = time.Now().Add(-d)
		}
		runs, err := probeRuns(ctx, l, q, limit)
		if err != nil {
			return err
		}
		slices.SortStableFunc(runs, func(a, b probeRun) int {
			return cmp.Compare(len(b.Report.Errors.Lost), len(a.Report.Errors.Lost))
		})
		worstRuns := runs[:min(worst, len(runs))]
		if asJSON {
			return writeJSON(map[string]any{"runs": len(runs), "by_family": byFamily(runs), "worst": worstRuns})
		}
		printProbes(runs, worstRuns)
		return nil
	})
}

// probeRuns checks each distinct original once, against the output its
// latest run produced; flushed originals are skipped, and so are proxy folds:
// their output points at an earlier result that still holds every fact.
func probeRuns(ctx context.Context, l *ledger.Ledger, q component.StatsQuery, limit int) ([]probeRun, error) {
	entries, err := l.Page(ctx, q, ledger.Page{Outcomes: []string{string(component.Optimized)}, Limit: limit})
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var runs []probeRun
	for _, e := range entries {
		if e.RawID == "" || e.Route == "fold" || seen[e.RawID] {
			continue
		}
		seen[e.RawID] = true
		_, orig, err := l.GetRaw(ctx, e.RawID)
		if err != nil {
			continue
		}
		out, ok, err := l.GetOutput(ctx, e.RawID)
		if err != nil || !ok {
			continue
		}
		family := "-"
		if len(e.Steps) > 0 {
			family = e.Steps[0].Family
		}
		runs = append(runs, probeRun{RawID: e.RawID, Command: e.Command, Family: family, Report: probe.Run(orig.Text(), out)})
	}
	return runs, nil
}

// kept counts facts of one kind across runs.
type kept struct {
	Total int `json:"total"`
	Kept  int `json:"kept"`
}

func (k *kept) add(t probe.Tally) {
	k.Total += t.Total
	k.Kept += t.Kept()
}

func (k kept) String() string {
	if k.Total == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d %s", k.Kept, k.Total, mcptool.Percent(k.Kept, k.Total))
}

type familyProbe struct {
	Family    string `json:"family"`
	Runs      int    `json:"runs"`
	Errors    kept   `json:"errors"`
	Artifacts kept   `json:"artifacts"`
	Numerics  kept   `json:"numerics"`
}

func byFamily(runs []probeRun) []familyProbe {
	m := map[string]*familyProbe{}
	for _, r := range runs {
		f := m[r.Family]
		if f == nil {
			f = &familyProbe{Family: r.Family}
			m[r.Family] = f
		}
		f.Runs++
		f.Errors.add(r.Report.Errors)
		f.Artifacts.add(r.Report.Artifacts)
		f.Numerics.add(r.Report.Numerics)
	}
	out := make([]familyProbe, 0, len(m))
	for _, f := range m {
		out = append(out, *f)
	}
	slices.SortFunc(out, func(a, b familyProbe) int {
		return cmp.Or(cmp.Compare(b.Runs, a.Runs), cmp.Compare(a.Family, b.Family))
	})
	return out
}

func printProbes(runs, worst []probeRun) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "%d optimized originals checked\n\n", len(runs))
	fmt.Fprintln(w, "family\truns\terror lines kept\tartifacts kept\tnumerics kept")
	for _, f := range byFamily(runs) {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\n", f.Family, f.Runs, f.Errors, f.Artifacts, f.Numerics)
	}
	w.Flush()
	if len(worst) == 0 || len(worst[0].Report.Errors.Lost) == 0 {
		return
	}
	fmt.Println("\nmost error lines lost (gascoigne raw <id> for the original):")
	for _, r := range worst {
		lost := r.Report.Errors.Lost
		if len(lost) == 0 {
			break
		}
		fmt.Printf("  %s  %-24s %d of %d, e.g. %s\n", r.RawID, r.Command, len(lost), r.Report.Errors.Total, lost[0])
	}
}
