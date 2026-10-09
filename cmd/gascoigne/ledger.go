package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/extension/ledger"
)

// withLedger parses flags, starts the service and hands the ledger to fn.
func withLedger(ctx context.Context, name string, args []string, nargs int, flags func(*flag.FlagSet),
	fn func(l *ledger.Ledger, args []string) error) error {
	var c common
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	c.register(fs)
	if flags != nil {
		flags(fs)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != nargs {
		return fmt.Errorf("%s: want %d argument(s), got %d", name, nargs, fs.NArg())
	}
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	defer svc.Shutdown(context.WithoutCancel(ctx))
	l, ok := svc.Ledger().(*ledger.Ledger)
	if !ok {
		return errors.New("no ledger extension configured")
	}
	return fn(l, fs.Args())
}

func cmdRaw(ctx context.Context, args []string) error {
	return withLedger(ctx, "raw", args, 1, nil, func(l *ledger.Ledger, args []string) error {
		key, p, err := l.GetRaw(ctx, args[0])
		if err != nil {
			return err
		}
		// Needing the original means the optimized output was not enough.
		if key != "" {
			if err := l.MarkFailure(ctx, key, component.ReasonRawRequested); err != nil {
				return err
			}
		}
		_, err = io.WriteString(os.Stdout, p.Text())
		return err
	})
}

func cmdFail(ctx context.Context, args []string) error {
	var reason string
	return withLedger(ctx, "fail", args, 1, func(fs *flag.FlagSet) {
		fs.StringVar(&reason, "reason", component.ReasonManual, "failure reason")
	}, func(l *ledger.Ledger, args []string) error {
		key := args[0]
		if !isKey(key) {
			k, _, err := l.GetRaw(ctx, key)
			if err != nil {
				return err
			}
			if k == "" {
				return fmt.Errorf("raw %s has no source key", key)
			}
			key = k
		}
		return l.MarkFailure(ctx, key, reason)
	})
}

func cmdForget(ctx context.Context, args []string) error {
	return withLedger(ctx, "forget", args, 1, nil, func(l *ledger.Ledger, args []string) error {
		return l.Forget(ctx, args[0])
	})
}

func isKey(s string) bool {
	for _, prefix := range []string{"file:", "cmd:", "tool:"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func cmdGain(ctx context.Context, args []string) error {
	var failures bool
	return withLedger(ctx, "gain", args, 0, func(fs *flag.FlagSet) {
		fs.BoolVar(&failures, "failures", false, "list sources that optimized badly")
	}, func(l *ledger.Ledger, _ []string) error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()
		if failures {
			return printFailures(ctx, w, l)
		}
		return printGain(ctx, w, l)
	})
}

func printGain(ctx context.Context, w io.Writer, l *ledger.Ledger) error {
	stats, err := l.Gain(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "route\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %\t")
	var total ledger.Stat
	for _, s := range stats {
		printStat(w, s)
		total.Runs += s.Runs
		total.Optimized += s.Optimized
		total.Failed += s.Failed
		total.RawTokens += s.RawTokens
		total.OutTokens += s.OutTokens
	}
	total.Route = "total"
	printStat(w, total)
	return nil
}

func printStat(w io.Writer, s ledger.Stat) {
	fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t\n",
		s.Route, s.Runs, s.Optimized, s.Failed, s.RawTokens, s.OutTokens, s.Saved(), percent(s.Saved(), s.RawTokens))
}

func percent(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(part)/float64(whole))
}

func printFailures(ctx context.Context, w io.Writer, l *ledger.Ledger) error {
	fs, err := l.FailureList(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "key\tcount\tlast\treason\t")
	for _, f := range fs {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t\n", f.Key, f.Count, f.Last.Format(time.DateTime), f.Reason)
	}
	return nil
}
