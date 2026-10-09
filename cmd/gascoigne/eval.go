package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/internal/extension/ledger"
	"github.com/lukaszb/gascoigne/internal/payload"
	"github.com/lukaszb/gascoigne/internal/registry"
)

type evalCase struct {
	File  string            `yaml:"file"`
	Kind  payload.Kind      `yaml:"kind"`
	Attrs map[string]string `yaml:"attrs"`
}

// cmdEval pushes fixtures through a pipeline (without exporters) against a
// throwaway ledger, reports savings and checks every original is retrievable.
func cmdEval(ctx context.Context, args []string) error {
	var c common
	fs := flag.NewFlagSet("eval", flag.ContinueOnError)
	c.register(fs)
	casesPath := fs.String("cases", "testdata/eval/cases.yaml", "eval cases file; fixture paths are relative to it")
	pipeline := fs.String("pipeline", "cli", "pipeline to evaluate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	data, err := os.ReadFile(*casesPath)
	if err != nil {
		return err
	}
	var cases []evalCase
	if err := config.Decode(data, &cases); err != nil {
		return fmt.Errorf("%s: %w", *casesPath, err)
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
	pl, ok := svc.Pipeline(*pipeline)
	if !ok {
		return fmt.Errorf("no pipeline %q", *pipeline)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "case\troute\traw tok\tout tok\tsaved %\traw ok")
	var rawTotal, outTotal int
	var errs []error
	for _, tc := range cases {
		text, err := os.ReadFile(filepath.Join(filepath.Dir(*casesPath), tc.File))
		if err != nil {
			return err
		}
		in := payload.New(tc.Kind, string(text))
		for k, v := range tc.Attrs {
			in.Set(k, v)
		}
		out, err := pl.Optimize(ctx, in)
		if err != nil {
			return fmt.Errorf("%s: %w", tc.File, err)
		}
		rawOK := "-"
		if id := out.Attrs[payload.AttrRawID]; id != "" {
			rawOK = "yes"
			if _, orig, err := svc.Ledger().GetRaw(ctx, id); err != nil || !orig.SameContent(in) {
				rawOK = "NO"
				errs = append(errs, fmt.Errorf("%s: original not retrievable byte-exact (%v)", tc.File, err))
			}
		}
		raw, opt := in.Tokens(), out.Tokens()
		rawTotal += raw
		outTotal += opt
		fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\t%s\n", tc.File, cmp.Or(out.Attrs[payload.AttrRoute], "-"), raw, opt, percent(raw-opt, raw), rawOK)
	}
	fmt.Fprintf(w, "total\t\t%d\t%d\t%s\t\n", rawTotal, outTotal, percent(rawTotal-outTotal, rawTotal))
	w.Flush()
	return errors.Join(errs...)
}

func cmdComponents(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("components", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()
	fmt.Fprintln(w, "kind\ttype")
	for _, f := range registry.Factories() {
		fmt.Fprintf(w, "%s\t%s\n", f.Kind(), f.Type())
	}
	return nil
}
