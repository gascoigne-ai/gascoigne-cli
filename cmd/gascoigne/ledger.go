package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/mcptool"
)

// withLedger parses flags, starts the service and hands the ledger to fn.
func withLedger(ctx context.Context, name string, args []string, nargs int, flags func(*flag.FlagSet),
	fn func(l *ledger.Ledger, c *common, args []string) error) error {
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
	return fn(l, &c, fs.Args())
}

func cmdRaw(ctx context.Context, args []string) error {
	return withLedger(ctx, "raw", args, 1, nil, func(l *ledger.Ledger, _ *common, args []string) error {
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
	}, func(l *ledger.Ledger, _ *common, args []string) error {
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
	return withLedger(ctx, "forget", args, 1, nil, func(l *ledger.Ledger, _ *common, args []string) error {
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
	var (
		failures bool
		by       string
		all      bool
		asJSON   bool
	)
	return withLedger(ctx, "gain", args, 0, func(fs *flag.FlagSet) {
		fs.BoolVar(&failures, "failures", false, "list sources that optimized badly")
		fs.StringVar(&by, "by", "", "group by route (default), session, family or optimizer")
		fs.BoolVar(&all, "all", false, "every session, even when -session or $GASCOIGNE_SESSION is set")
		fs.BoolVar(&asJSON, "json", false, "print JSON instead of a table")
	}, func(l *ledger.Ledger, c *common, _ []string) error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()
		if failures {
			if asJSON {
				return failuresJSON(ctx, l)
			}
			return printFailures(ctx, w, l)
		}
		q := component.StatsQuery{By: by, Session: c.sessionID()}
		if all {
			q.Session = ""
		}
		stats, err := l.Stats(ctx, q)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(mcptool.Report(q, stats))
		}
		if q.Session != "" {
			fmt.Printf("session %s\n", q.Session)
		}
		_, err = io.WriteString(os.Stdout, mcptool.StatsTable(q.By, stats))
		return err
	})
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

func failuresJSON(ctx context.Context, l *ledger.Ledger) error {
	fs, err := l.FailureList(ctx)
	if err != nil {
		return err
	}
	type failure struct {
		Key    string    `json:"key"`
		Count  int       `json:"count"`
		Last   time.Time `json:"last"`
		Reason string    `json:"reason"`
	}
	out := []failure{}
	for _, f := range fs {
		out = append(out, failure{f.Key, f.Count, f.Last, f.Reason})
	}
	return writeJSON(map[string]any{"failures": out})
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
