package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/probe"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/internal/prose"
	"github.com/gascoigne-ai/gascoigne-cli/internal/registry"
	"github.com/gascoigne-ai/gascoigne-cli/internal/service"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type evalBlock struct {
	File string       `yaml:"file"`
	Kind payload.Kind `yaml:"kind"`
}

type evalCase struct {
	Name  string            `yaml:"name"`
	File  string            `yaml:"file"`
	Kind  payload.Kind      `yaml:"kind"`
	Attrs map[string]string `yaml:"attrs"`
	// Blocks builds a multi-block payload instead of File.
	Blocks []evalBlock `yaml:"blocks"`
	// ExpectType is the detected type of the blocks ("a+b" for several kinds).
	ExpectType string `yaml:"expect_type"`
	// Keep lists substrings that must survive optimization.
	Keep []string `yaml:"keep"`
	// MinSaved is the lowest acceptable saving in percent.
	MinSaved float64 `yaml:"min_saved"`
	// MaxMS is the slowest acceptable optimization in milliseconds (wall
	// time of one run; scale cases use it to catch slowdowns). 0 allows any.
	MaxMS int `yaml:"max_ms"`
	// Unchanged inputs must pass through as they are.
	Unchanged bool `yaml:"unchanged"`
	// Lossless cases may shrink but lose no error line or artifact (probe.Run).
	Lossless bool `yaml:"lossless"`
	// Run is a shell command whose live output replaces File: it catches
	// tools whose output changed shape. Its output varies, so it has no
	// golden or baseline; a missing program skips the case.
	Run string `yaml:"run"`
	// Golden keeps a golden file for a detect/ case, which otherwise checks
	// only its detected type: the typed case of the same fixture holds the output.
	Golden bool `yaml:"golden"`
	// Reference names a file with the text the output must still hold (an
	// article's body for a web page); eval reports word recall and
	// precision against it, and fails under MinRecall (%).
	Reference string  `yaml:"reference"`
	MinRecall float64 `yaml:"min_recall"`
}

func (c *evalCase) name() string {
	return cmp.Or(c.Name, strings.TrimSuffix(c.File, filepath.Ext(c.File)), c.Run)
}

type evalFlags struct {
	cases, pipeline, show string
	update, record        bool
}

// cmdEval pushes fixtures through a pipeline (without exporters) against a
// throwaway ledger, reports savings and checks assertions, goldens and that
// every original is retrievable byte-exact.
func cmdEval(ctx context.Context, args []string) error {
	var cfg common
	ef, err := parseEvalFlags(&cfg, args)
	if err != nil {
		return err
	}
	cases, err := loadEvalCases(ef.cases)
	if err != nil {
		return err
	}
	if err := checkProseInstalled(cfg.sets); err != nil {
		return err
	}
	cleanup, err := chooseEvalLedger(&cfg, ef)
	if err != nil {
		return err
	}
	defer cleanup()

	svc, err := cfg.start(ctx)
	if err != nil {
		return err
	}
	defer svc.Shutdown(context.WithoutCancel(ctx))
	pl, ok := svc.Pipeline(ef.pipeline)
	if !ok {
		return fmt.Errorf("no pipeline %q", ef.pipeline)
	}

	return runEval(ctx, svc, pl, cases, ef)
}

// runEval runs every case through pl, compares with the baseline file next to
// the cases and reports.
func runEval(ctx context.Context, svc *service.Service, pl *service.Pipeline, cases []evalCase, ef evalFlags) error {
	dir := filepath.Dir(ef.cases)
	basePath := filepath.Join(dir, "baseline.tsv")
	base, err := readBaseline(basePath)
	if err != nil {
		return err
	}
	paint, err := newPainter("auto")
	if err != nil {
		return err
	}
	run := evalRun{base: base, next: map[string]baseline{}, ef: ef}
	for _, tc := range cases {
		r, err := runCase(ctx, svc, pl, dir, &tc, ef.update)
		if errors.Is(err, errNotInstalled) {
			run.skip(&tc)
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", tc.name(), err)
		}
		run.add(&tc, r)
	}
	return run.finish(paint, basePath)
}

func parseEvalFlags(cfg *common, args []string) (evalFlags, error) {
	var ef evalFlags
	fs := newFlags("eval")
	cfg.register(fs)
	cfg.plain = true
	fs.StringVar(&ef.cases, "cases", "testdata/eval/cases.yaml", "eval cases `file`; fixture paths are relative to it")
	fs.StringVar(&ef.pipeline, "pipeline", "cli", "`pipeline` to evaluate")
	fs.StringVar(&ef.show, "show", "", "print optimized output of cases whose name contains `text`")
	fs.BoolVar(&ef.update, "update", false, "rewrite golden outputs")
	fs.BoolVar(&ef.record, "record", false, "write the runs to your ledger instead of a throwaway one, so the dashboard shows them (agent eval, one session per run, and the dashboard server is started; filter or `reset -agent eval` to drop them)")
	return ef, fs.Parse(args)
}

func loadEvalCases(path string) ([]evalCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []evalCase
	if err := config.Decode(data, &cases); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return cases, nil
}

// chooseEvalLedger points the run at the user's ledger (-record) or at a
// throwaway one; the returned func removes the throwaway.
func chooseEvalLedger(cfg *common, ef evalFlags) (cleanup func(), err error) {
	if ef.record {
		cfg.server = true // so the dashboard is there to show them
		cfg.agent = cmp.Or(cfg.agent, "eval")
		cfg.session = cmp.Or(cfg.session, "eval-"+time.Now().Format("20060102-150405"))
		return func() {}, nil
	}
	tmp, err := os.MkdirTemp("", "gascoigne-eval-")
	if err != nil {
		return nil, err
	}
	os.Setenv(ledger.EnvPath, filepath.Join(tmp, "ledger.db"))
	return func() { os.RemoveAll(tmp) }, nil
}

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

type caseResult struct {
	types, route, text string
	raw, out           int
	took               time.Duration
	overlap            *overlap // nil without a reference
	probes             probe.Report
	problems           []string
}

func runCase(ctx context.Context, svc *service.Service, pl *service.Pipeline, dir string, tc *evalCase, update bool) (*caseResult, error) {
	in, err := tc.payload(ctx, dir)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	out, err := pl.Optimize(ctx, in)
	if err != nil {
		return nil, err
	}
	took := time.Since(start)
	again, err := pl.Optimize(ctx, in)
	if err != nil {
		return nil, err
	}
	r := &caseResult{
		types: blockTypes(out),
		route: cmp.Or(out.Attrs[payload.AttrRoute], "-"),
		text:  out.Text(),
		raw:   in.Tokens(),
		out:   out.Tokens(),
		took:  took,
	}
	r.probes = probe.Run(in.Text(), r.text)
	if again.Text() != r.text {
		r.fail("nondeterministic: two runs differ")
	}
	r.checkExpectations(ctx, svc, tc, in, out)
	if tc.Reference != "" {
		if err := r.checkReference(dir, tc); err != nil {
			return nil, err
		}
	}
	if tc.Run != "" {
		return r, nil
	}
	return r, r.checkGolden(dir, tc, out, update)
}

func (r *caseResult) fail(format string, args ...any) {
	r.problems = append(r.problems, fmt.Sprintf(format, args...))
}

// checkExpectations records a problem for every case setting the output breaks:
// retrievable original, type, kept text, size, savings and time.
func (r *caseResult) checkExpectations(ctx context.Context, svc *service.Service, tc *evalCase, in, out *payload.Payload) {
	if id := out.Attrs[payload.AttrRawID]; id != "" {
		if _, orig, err := svc.Ledger().GetRaw(ctx, id); err != nil || !sameText(orig, in) {
			r.fail("original not retrievable byte-exact (%v)", err)
		}
	}
	if tc.ExpectType != "" && r.types != tc.ExpectType {
		r.fail("type %s, want %s", r.types, tc.ExpectType)
	}
	for _, k := range tc.Keep {
		if !strings.Contains(r.text, k) {
			r.fail("lost %q", k)
		}
	}
	if r.out > r.raw {
		r.fail("output larger than input: %d > %d tokens", r.out, r.raw)
	}
	if tc.Unchanged && out.Attrs[payload.AttrRawID] != "" {
		r.fail("changed, want unchanged")
	}
	if lost := slices.Concat(r.probes.Errors.Lost, r.probes.Artifacts.Lost); tc.Lossless && len(lost) > 0 {
		r.fail("lost %d error lines/artifacts, want none: %q", len(lost), lost[0])
	}
	if saved := 100 * float64(r.raw-r.out) / float64(max(r.raw, 1)); saved < tc.MinSaved {
		r.fail("saved %.1f%% < min %.0f%%", saved, tc.MinSaved)
	}

	if slow(r.took, tc.MaxMS) {
		r.fail("took %s > max %d ms", ms(r.took), tc.MaxMS)
	}
}

func (r *caseResult) checkReference(dir string, tc *evalCase) error {
	ref, err := os.ReadFile(filepath.Join(dir, tc.Reference))
	if err != nil {
		return err
	}
	o := wordOverlap(string(ref), r.text)
	r.overlap = &o
	if recall := 100 * o.recall(); recall < tc.MinRecall {
		r.fail("reference recall %.1f%% < min %.0f%%", recall, tc.MinRecall)
	}
	return nil
}

// checkGolden compares the output with the case's golden file, or rewrites
// the golden when update is set.
func (r *caseResult) checkGolden(dir string, tc *evalCase, out *payload.Payload, update bool) error {
	golden := filepath.Join(dir, "golden", tc.name()+".out")
	if strings.HasPrefix(tc.name(), "detect/") && !tc.Golden {
		if update {
			os.Remove(golden)
		}
		return nil
	}
	if update {
		// Unchanged output needs no golden: the fixture is the expectation.
		if out.Attrs[payload.AttrRawID] == "" {
			os.Remove(golden)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			return err
		}
		return os.WriteFile(golden, []byte(r.text), 0o644)
	}
	if want, err := os.ReadFile(golden); err == nil && !bytes.Equal(want, []byte(r.text)) {
		r.fail("differs from golden %s (review, then rerun with -update)", golden)
	}
	return nil
}

// slow reports whether took exceeds a case's max_ms; 0 allows any time.
func slow(took time.Duration, maxMS int) bool {
	return maxMS > 0 && took > time.Duration(maxMS)*time.Millisecond
}

// sameText compares block texts only: detection fills in Block.Type on the stored original.
func sameText(left, right *payload.Payload) bool {
	return slices.EqualFunc(left.Blocks, right.Blocks, func(leftBlock, rightBlock payload.Block) bool {
		return leftBlock.Kind == rightBlock.Kind && leftBlock.Text == rightBlock.Text
	})
}

// errNotInstalled: a live case's program is missing here.
var errNotInstalled = errors.New("not installed")

func (c *evalCase) payload(ctx context.Context, dir string) (*payload.Payload, error) {
	if c.Run != "" {
		return c.live(ctx)
	}
	blocks := c.Blocks
	if len(blocks) == 0 {
		blocks = []evalBlock{{c.File, c.Kind}}
	}
	p := &payload.Payload{Attrs: map[string]string{}}
	for _, b := range blocks {
		text, err := os.ReadFile(filepath.Join(dir, b.File))
		if err != nil {
			return nil, err
		}
		p.Blocks = append(p.Blocks, payload.Block{Kind: cmp.Or(b.Kind, payload.CmdOutput), Text: string(text)})
	}
	for k, v := range c.Attrs {
		p.Set(k, v)
	}
	return p, nil
}

func blockTypes(content *payload.Payload) string {
	var types []string
	for _, b := range content.Blocks {
		types = append(types, cmp.Or(b.Type, "-"))
	}
	return strings.Join(slices.Compact(types), "+")
}

func cmdComponents(_ context.Context, args []string) error {
	fs := newFlags("components")
	if err := fs.Parse(args); err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "kind\tfamily\ttype")
	for _, f := range registry.Factories() {
		fmt.Fprintf(w, "%s\t%s\t%s\n", f.Kind(), cmp.Or(f.Family(), "-"), f.Type())
	}
	return nil
}

// live runs the case's command in the working directory: stdout and
// stderr as an agent sees them.
func (c *evalCase) live(ctx context.Context) (*payload.Payload, error) {
	out, err := exec.CommandContext(ctx, "sh", "-c", c.Run).CombinedOutput()
	// A failing command is a case too (test failures, build errors); only
	// "command not found" (127) or not starting at all is not.
	exit, failed := errors.AsType[*exec.ExitError](err)
	if failed && exit.ExitCode() == 127 {
		return nil, errNotInstalled
	}
	if err != nil && !failed {
		return nil, err
	}
	kind := cmp.Or(c.Kind, payload.CmdOutput)
	p := &payload.Payload{Blocks: []payload.Block{{Kind: kind, Text: string(out)}}, Attrs: map[string]string{}}
	if kind == payload.CmdOutput { // a tool result carries the tool, as the proxy sets it, not the command that made it
		p.Set(payload.AttrCmd, c.Run)
	}
	for k, v := range c.Attrs {
		p.Set(k, v)
	}
	return p, nil
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

func (o overlap) recall() float64    { return float64(o.both) / float64(max(o.ref, 1)) }
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

// checkProseInstalled stops a run that turns the prose optimizer on without
// its model: that run would pass text through and fail as golden diffs.
func checkProseInstalled(sets []string) error {
	if !slices.Contains(sets, "optimizers.prose.enabled=true") {
		return nil
	}
	dir, _ := prose.ModelDir()
	name := configProseModel()
	for _, s := range sets {
		if v, ok := strings.CutPrefix(s, "optimizers.prose.model="); ok {
			name = v
		}
	}
	if model, rt := prose.Installed(dir, name); !model || !rt {
		return fmt.Errorf("prose model %q not installed: run gascoigne prose install -name %s", name, name)
	}
	return nil
}
