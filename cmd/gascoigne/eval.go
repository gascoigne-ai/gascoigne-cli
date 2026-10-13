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

	"github.com/gascoigne-ai/gascoigne-cli/compress/probe"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/registry"
	"github.com/gascoigne-ai/gascoigne-cli/internal/service"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/prose"
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
	// Unchanged inputs must pass through as they are.
	Unchanged bool `yaml:"unchanged"`
	// Lossless cases may shrink but lose no error line or artifact (probe.Run).
	Lossless bool `yaml:"lossless"`
	// Run is a shell command whose live output replaces File: it catches
	// tools whose output changed shape. Its output varies, so it has no
	// golden or baseline; a missing program skips the case.
	Run string `yaml:"run"`
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
	var (
		c  common
		ef evalFlags
	)
	fs := newFlags("eval")
	c.register(fs)
	c.plain = true
	fs.StringVar(&ef.cases, "cases", "testdata/eval/cases.yaml", "eval cases `file`; fixture paths are relative to it")
	fs.StringVar(&ef.pipeline, "pipeline", "cli", "`pipeline` to evaluate")
	fs.StringVar(&ef.show, "show", "", "print optimized output of cases whose name contains `text`")
	fs.BoolVar(&ef.update, "update", false, "rewrite golden outputs")
	fs.BoolVar(&ef.record, "record", false, "write the runs to your ledger instead of a throwaway one, so the dashboard shows them (agent eval, one session per run, and the dashboard server is started; filter or `reset -agent eval` to drop them)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(ef.cases)
	if err != nil {
		return err
	}
	var cases []evalCase
	if err := config.Decode(data, &cases); err != nil {
		return fmt.Errorf("%s: %w", ef.cases, err)
	}
	if err := checkProseInstalled(c.sets); err != nil {
		return err
	}

	if ef.record {
		c.server = true // so the dashboard is there to show them
		c.agent = cmp.Or(c.agent, "eval")
		c.session = cmp.Or(c.session, "eval-"+time.Now().Format("20060102-150405"))
	} else {
		tmp, err := os.MkdirTemp("", "gascoigne-eval-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		os.Setenv(ledger.EnvPath, filepath.Join(tmp, "ledger.db"))
	}

	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	defer svc.Shutdown(context.WithoutCancel(ctx))
	pl, ok := svc.Pipeline(ef.pipeline)
	if !ok {
		return fmt.Errorf("no pipeline %q", ef.pipeline)
	}

	dir := filepath.Dir(ef.cases)
	basePath := filepath.Join(dir, "baseline.tsv")
	base, err := readBaseline(basePath)
	if err != nil {
		return err
	}
	next := map[string]baseline{}
	var names []string
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "case\ttype\troute\traw tok\tout tok\tsaved\tlost\tms\tcheck")
	var rawTotal, outTotal int
	var took time.Duration
	var errs, shows []string
	var refs []overlap
	for _, tc := range cases {
		r, err := runCase(ctx, svc, pl, dir, &tc, ef.update)
		if errors.Is(err, errNotInstalled) {
			fmt.Fprintf(w, "%s\t\t\t\t\t\t\t\tskip: not installed\n", tc.name())
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", tc.name(), err)
		}
		rawTotal += r.raw
		outTotal += r.out
		if r.overlap != nil {
			refs = append(refs, *r.overlap)
		}
		took += r.took
		cur := r.baseline()
		if tc.Run == "" {
			names = append(names, tc.name())
			next[tc.name()] = cur
		}
		if old, ok := base[tc.name()]; base != nil && !ef.update && tc.Run == "" {
			if !ok {
				r.problems = append(r.problems, "not in baseline (rerun with -update)")
			} else if d := cur.diff(old); d != "" {
				r.problems = append(r.problems, "baseline: "+d+" (review, then rerun with -update)")
			}
		}
		check := "ok"
		if len(r.problems) > 0 {
			check = "FAIL"
			for _, p := range r.problems {
				errs = append(errs, tc.name()+": "+p)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n", tc.name(), r.types, r.route, r.raw, r.out, mcptool.Percent(r.raw-r.out, r.raw), lostCell(r.probes), ms(r.took), check)
		if ef.show != "" && strings.Contains(tc.name(), ef.show) {
			shows = append(shows, fmt.Sprintf("=== %s ===\n%s", tc.name(), r.text))
		}
	}
	fmt.Fprintf(w, "total\t\t\t%d\t%d\t%s\t\t%s\t\n", rawTotal, outTotal, mcptool.Percent(rawTotal-outTotal, rawTotal), ms(took))
	w.Flush()
	if len(refs) > 0 {
		var recall, precision float64
		for _, o := range refs {
			recall += o.recall()
			precision += o.precision()
		}
		n := float64(len(refs))
		fmt.Printf("reference words, mean of %d cases: recall %.1f%%, precision %.1f%%\n", len(refs), 100*recall/n, 100*precision/n)
	}
	if ef.update && len(names) > 0 {
		if err := writeBaseline(basePath, names, next); err != nil {
			return err
		}
	}
	for _, s := range shows {
		fmt.Println(s)
	}
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "\nfailures:\n  "+strings.Join(errs, "\n  "))
		return errors.New("eval failed")
	}
	return nil
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
	fail := func(format string, args ...any) { r.problems = append(r.problems, fmt.Sprintf(format, args...)) }
	if again.Text() != r.text {
		fail("nondeterministic: two runs differ")
	}

	if id := out.Attrs[payload.AttrRawID]; id != "" {
		if _, orig, err := svc.Ledger().GetRaw(ctx, id); err != nil || !sameText(orig, in) {
			fail("original not retrievable byte-exact (%v)", err)
		}
	}
	if tc.ExpectType != "" && r.types != tc.ExpectType {
		fail("type %s, want %s", r.types, tc.ExpectType)
	}
	for _, k := range tc.Keep {
		if !strings.Contains(r.text, k) {
			fail("lost %q", k)
		}
	}
	if r.out > r.raw {
		fail("output larger than input: %d > %d tokens", r.out, r.raw)
	}
	if tc.Unchanged && out.Attrs[payload.AttrRawID] != "" {
		fail("changed, want unchanged")
	}
	if lost := slices.Concat(r.probes.Errors.Lost, r.probes.Artifacts.Lost); tc.Lossless && len(lost) > 0 {
		fail("lost %d error lines/artifacts, want none: %q", len(lost), lost[0])
	}
	if saved := 100 * float64(r.raw-r.out) / float64(max(r.raw, 1)); saved < tc.MinSaved {
		fail("saved %.1f%% < min %.0f%%", saved, tc.MinSaved)
	}

	if tc.Reference != "" {
		ref, err := os.ReadFile(filepath.Join(dir, tc.Reference))
		if err != nil {
			return nil, err
		}
		o := wordOverlap(string(ref), r.text)
		r.overlap = &o
		if recall := 100 * o.recall(); recall < tc.MinRecall {
			fail("reference recall %.1f%% < min %.0f%%", recall, tc.MinRecall)
		}
	}
	if tc.Run != "" {
		return r, nil
	}
	golden := filepath.Join(dir, "golden", tc.name()+".out")
	if update {
		// Unchanged output needs no golden: the fixture is the expectation.
		if out.Attrs[payload.AttrRawID] == "" {
			os.Remove(golden)
			return r, nil
		}
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			return nil, err
		}
		return r, os.WriteFile(golden, []byte(r.text), 0o644)
	}
	if want, err := os.ReadFile(golden); err == nil && !bytes.Equal(want, []byte(r.text)) {
		fail("differs from golden %s (review, then rerun with -update)", golden)
	}
	return r, nil
}

// sameText compares block texts only: detection fills in Block.Type on the stored original.
func sameText(a, b *payload.Payload) bool {
	return slices.EqualFunc(a.Blocks, b.Blocks, func(x, y payload.Block) bool { return x.Kind == y.Kind && x.Text == y.Text })
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

func blockTypes(p *payload.Payload) string {
	var types []string
	for _, b := range p.Blocks {
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
	p := &payload.Payload{Blocks: []payload.Block{{Kind: payload.CmdOutput, Text: string(out)}}, Attrs: map[string]string{payload.AttrCmd: c.Run}}
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
	count := func(s string) map[string]int {
		m := map[string]int{}
		for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
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
func lostCell(r probe.Report) string {
	if len(r.Errors.Lost)+len(r.Artifacts.Lost) == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", len(r.Errors.Lost), len(r.Artifacts.Lost))
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

func writeBaseline(path string, names []string, m map[string]baseline) error {
	var sb strings.Builder
	sb.WriteString("# case\tout tokens\tlost errors\tlost artifacts (gascoigne eval -update)\n")
	for _, n := range names {
		b := m[n]
		fmt.Fprintf(&sb, "%s\t%d\t%d\t%d\n", n, b.out, b.errors, b.artifacts)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

// ms shows a duration in milliseconds: wall time, so it varies run to run
// and stays out of the baseline.
func ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d)/float64(time.Millisecond)) }

// checkProseInstalled stops a run that turns the prose optimizer on without
// its model: that run would pass text through and fail as golden diffs.
func checkProseInstalled(sets []string) error {
	if !slices.Contains(sets, "optimizers.prose.enabled=true") {
		return nil
	}
	dir, _ := prose.ModelDir()
	if model, rt := prose.Installed(dir); !model || !rt {
		return errors.New("prose model not installed: run gascoigne prose install")
	}
	return nil
}
