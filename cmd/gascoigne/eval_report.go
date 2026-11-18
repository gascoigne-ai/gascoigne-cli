package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/probe"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// evalRun accumulates the table rows, totals and failures of one eval run.
type evalRun struct {
	ef       evalFlags
	base     map[string]baseline
	next     map[string]baseline
	names    []string
	rows     [][]cell
	skipped  int
	rawTotal int
	outTotal int
	took     time.Duration
	errs     []string
	shows    []string
	refs     []overlap
}

func (run *evalRun) skip(tc *evalCase) {
	run.skipped++
	run.rows = append(run.rows, []cell{{text: tc.name()}, {}, {}, {}, {}, {}, {}, {}, {}, {text: "skip: not installed", style: dim}})
}

func (run *evalRun) add(tc *evalCase, res *caseResult) {
	run.rawTotal += res.raw
	run.outTotal += res.out
	if res.overlap != nil {
		run.refs = append(run.refs, *res.overlap)
	}
	run.took += res.took
	cur := res.baseline()
	if tc.Run == "" {
		run.names = append(run.names, tc.name())
		run.next[tc.name()] = cur
	}
	if old, ok := run.base[tc.name()]; run.base != nil && !run.ef.update && tc.Run == "" {
		if !ok {
			res.problems = append(res.problems, "not in baseline (rerun with -update)")
		} else if d := cur.diff(old); d != "" {
			res.problems = append(res.problems, "baseline: "+d+" (review, then rerun with -update)")
		}
	}
	check, checkStyle := "ok", green
	if len(res.problems) > 0 {
		check, checkStyle = "FAIL", bold+";"+red
		for _, p := range res.problems {
			run.errs = append(run.errs, tc.name()+": "+p)
		}
	}
	saved := pct(res.raw-res.out, res.raw)
	run.rows = append(run.rows, []cell{
		{text: tc.name()}, {text: res.types}, {text: res.route, style: dim},
		{text: fmt.Sprint(res.raw), right: true}, {text: fmt.Sprint(res.out), right: true},
		{text: mcptool.Percent(res.raw-res.out, res.raw), style: savedStyle(saved), right: true},
		{text: bar(saved/100, 10), style: savedStyle(saved)},
		{text: lostCell(res.probes), right: true}, {text: ms(res.took), right: true},
		{text: check, style: checkStyle},
	})
	if run.ef.show != "" && strings.Contains(tc.name(), run.ef.show) {
		run.shows = append(run.shows, fmt.Sprintf("=== %s ===\n%s", tc.name(), res.text))
	}
}

// finish prints the report, saves the baseline on -update and turns failures
// into the run's error.
func (run *evalRun) finish(paint painter, basePath string) error {
	run.printReport(paint)
	if run.ef.update && len(run.names) > 0 {
		if err := writeBaseline(basePath, run.names, run.next); err != nil {
			return err
		}
	}
	for _, s := range run.shows {
		fmt.Println(s)
	}
	if len(run.errs) > 0 {
		fmt.Fprintln(os.Stderr, "\nfailures:\n  "+strings.Join(run.errs, "\n  "))
		return errors.New("eval failed")
	}
	return nil
}

// printReport prints the table with its total row, the summary line and the
// reference-word means.
func (run *evalRun) printReport(paint painter) {
	header := []string{"case", "type", "route", "raw tok", "out tok", "saved", "", "lost", "ms", "check"}
	totalSaved := pct(run.rawTotal-run.outTotal, run.rawTotal)
	rows := append(run.rows, []cell{
		{text: "total", style: bold}, {}, {},
		{text: fmt.Sprint(run.rawTotal), right: true, style: bold}, {text: fmt.Sprint(run.outTotal), right: true, style: bold},
		{text: mcptool.Percent(run.rawTotal-run.outTotal, run.rawTotal), style: bold + ";" + savedStyle(totalSaved), right: true},
		{text: bar(totalSaved/100, 10), style: savedStyle(totalSaved)},
		{}, {text: ms(run.took), right: true, style: bold}, {},
	})
	paint.table(os.Stdout, header, rows)
	ran := len(rows) - 1 - run.skipped
	summary := fmt.Sprintf("%d cases: %d ok", ran, ran-len(run.errs))
	if len(run.errs) > 0 {
		summary = fmt.Sprintf("%d cases: %d ok, %s", ran, ran-len(run.errs), paint.paint(bold+";"+red, fmt.Sprintf("%d FAILED", len(run.errs))))
	}
	if run.skipped > 0 {
		summary += fmt.Sprintf(", %d skipped (tool not installed)", run.skipped)
	}
	fmt.Println(summary)
	if len(run.refs) > 0 {
		var recall, precision float64
		for _, o := range run.refs {
			recall += o.recall()
			precision += o.precision()
		}
		n := float64(len(run.refs))
		fmt.Printf("reference words, mean of %d cases: recall %.1f%%, precision %.1f%%\n", len(run.refs), 100*recall/n, 100*precision/n)
	}
}

// overlap counts the words a reference and an output share, each word as
// often as both have it: what token-level recall and precision measure.
type overlap struct{ ref, out, both int }

var wordRe = lazyre.New(`[\pL\pN]+`)

func wordOverlap(ref, out string) overlap {
	count := func(text string) map[string]int {
		m := map[string]int{}
		for _, w := range wordRe.FindAllString(strings.ToLower(text), -1) {
			m[w]++
		}
		return m
	}
	r, o := count(ref), count(out)
	var v overlap
	for w, n := range r {
		v.ref += n
		v.both += min(n, o[w])
	}
	for _, n := range o {
		v.out += n
	}
	return v
}

func (o overlap) recall() float64 { return float64(o.both) / float64(max(o.ref, 1)) }

func (o overlap) precision() float64 { return float64(o.both) / float64(max(o.out, 1)) }

// lostCell shows lost error lines and artifacts as "errors/artifacts", "-" when none.
func lostCell(report probe.Report) string {
	if len(report.Errors.Lost)+len(report.Artifacts.Lost) == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", len(report.Errors.Lost), len(report.Artifacts.Lost))
}

// baseline is what a case measured when last accepted: optimized tokens and
// facts lost (probe.Run). Kept in baseline.tsv next to the cases, so a change
// in savings or retention shows in the diff and fails until accepted.
type baseline struct{ out, errors, artifacts int }

func (r *caseResult) baseline() baseline {
	return baseline{r.out, len(r.probes.Errors.Lost), len(r.probes.Artifacts.Lost)}
}

// diff describes what changed since old, "" when nothing did.
func (b baseline) diff(old baseline) string {
	var d []string
	for _, f := range []struct {
		what     string
		cur, old int
	}{{"out tokens", b.out, old.out}, {"lost errors", b.errors, old.errors}, {"lost artifacts", b.artifacts, old.artifacts}} {
		if f.cur != f.old {
			d = append(d, fmt.Sprintf("%s %d→%d", f.what, f.old, f.cur))
		}
	}
	return strings.Join(d, ", ")
}

// readBaseline returns nil without error when there is no baseline file.
func readBaseline(path string) (map[string]baseline, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]baseline{}
	for i, l := range strings.Split(string(data), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		var name string
		var b baseline
		if _, err := fmt.Sscanf(l, "%s %d %d %d", &name, &b.out, &b.errors, &b.artifacts); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, i+1, err)
		}
		m[name] = b
	}
	return m, nil
}

func writeBaseline(path string, names []string, baselines map[string]baseline) error {
	var sb strings.Builder
	sb.WriteString("# case\tout tokens\tlost errors\tlost artifacts (gascoigne eval -update)\n")
	for _, n := range names {
		b := baselines[n]
		fmt.Fprintf(&sb, "%s\t%d\t%d\t%d\n", n, b.out, b.errors, b.artifacts)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// ms shows a duration in milliseconds: wall time, so it varies run to run
// and stays out of the baseline.
func ms(duration time.Duration) string {
	return fmt.Sprintf("%.1f", float64(duration)/float64(time.Millisecond))
}
