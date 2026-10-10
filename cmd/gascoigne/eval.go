package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/internal/registry"
	"github.com/lukaszb/gascoigne/internal/service"
	"github.com/lukaszb/gascoigne/mcptool"
	"github.com/lukaszb/gascoigne/payload"
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
}

func (c *evalCase) name() string {
	return cmp.Or(c.Name, strings.TrimSuffix(c.File, filepath.Ext(c.File)))
}

type evalFlags struct {
	cases, pipeline, show string
	update                bool
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
	fs.StringVar(&ef.cases, "cases", "testdata/eval/cases.yaml", "eval cases `file`; fixture paths are relative to it")
	fs.StringVar(&ef.pipeline, "pipeline", "cli", "`pipeline` to evaluate")
	fs.StringVar(&ef.show, "show", "", "print optimized output of cases whose name contains `text`")
	fs.BoolVar(&ef.update, "update", false, "rewrite golden outputs")
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

	tmp, err := os.MkdirTemp("", "gascoigne-eval-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	os.Setenv(ledger.EnvPath, filepath.Join(tmp, "ledger.db"))

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
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "case\ttype\troute\traw tok\tout tok\tsaved\tcheck")
	var rawTotal, outTotal int
	var errs, shows []string
	for _, tc := range cases {
		r, err := runCase(ctx, svc, pl, dir, &tc, ef.update)
		if err != nil {
			return fmt.Errorf("%s: %w", tc.name(), err)
		}
		rawTotal += r.raw
		outTotal += r.out
		check := "ok"
		if len(r.problems) > 0 {
			check = "FAIL"
			for _, p := range r.problems {
				errs = append(errs, tc.name()+": "+p)
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n", tc.name(), r.types, r.route, r.raw, r.out, mcptool.Percent(r.raw-r.out, r.raw), check)
		if ef.show != "" && strings.Contains(tc.name(), ef.show) {
			shows = append(shows, fmt.Sprintf("=== %s ===\n%s", tc.name(), r.text))
		}
	}
	fmt.Fprintf(w, "total\t\t\t%d\t%d\t%s\t\n", rawTotal, outTotal, mcptool.Percent(rawTotal-outTotal, rawTotal))
	w.Flush()
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
	problems           []string
}

func runCase(ctx context.Context, svc *service.Service, pl *service.Pipeline, dir string, tc *evalCase, update bool) (*caseResult, error) {
	in, err := tc.payload(dir)
	if err != nil {
		return nil, err
	}
	out, err := pl.Optimize(ctx, in)
	if err != nil {
		return nil, err
	}
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
	}
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
	if saved := 100 * float64(r.raw-r.out) / float64(max(r.raw, 1)); saved < tc.MinSaved {
		fail("saved %.1f%% < min %.0f%%", saved, tc.MinSaved)
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

func (c *evalCase) payload(dir string) (*payload.Payload, error) {
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
