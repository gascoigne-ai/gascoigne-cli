package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// withLedger parses flags, starts the service and hands the ledger to fn.
func withLedger(ctx context.Context, name string, args []string, nargs int, flags func(*flag.FlagSet),
	fn func(l *ledger.Ledger, c *common, args []string) error) error {
	var c common
	fs := newFlags(name)
	c.register(fs)
	if flags != nil {
		flags(fs)
	}
	// Flags may follow arguments too: agents append them to a copied hint
	// (gascoigne raw <id> -grep FAIL).
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(pos) != nargs {
		return fmt.Errorf("%s: want %d argument(s), got %d", name, nargs, len(pos))
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
	return fn(l, &c, pos)
}

func cmdRaw(ctx context.Context, args []string) error {
	var sl mcptool.Slice
	return withLedger(ctx, "raw", args, 1, func(fs *flag.FlagSet) {
		fs.StringVar(&sl.Lines, "lines", "", "only lines `a-b` (1-based, inclusive)")
		fs.StringVar(&sl.Grep, "grep", "", "only lines matching `regexp`, numbered")
		fs.StringVar(&sl.Query, "query", "", "only the lines best matching these `words`, numbered")
		fs.IntVar(&sl.Context, "C", 0, "lines of context around grep/query matches")
	}, func(l *ledger.Ledger, _ *common, args []string) error {
		text, err := mcptool.Original(ctx, l, args[0], sl)
		if err != nil {
			return err
		}
		_, err = io.WriteString(os.Stdout, text)
		return err
	})
}

func cmdFail(ctx context.Context, args []string) error {
	var reason string
	return withLedger(ctx, "fail", args, 1, func(fs *flag.FlagSet) {
		fs.StringVar(&reason, "reason", component.ReasonManual, "failure `reason`")
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
		oneline  bool
		color    string
		since    string
		rereads  float64
	)
	return withLedger(ctx, "gain", args, 0, func(fs *flag.FlagSet) {
		fs.BoolVar(&failures, "failures", false, "list sources that optimized badly")
		fs.StringVar(&by, "by", "", "one `grouping` instead of the overview: day, week, month, agent, model, session, command, optimizer, family, dialect, outcome, reason, route")
		fs.BoolVar(&all, "all", false, "ignore session/agent/model from $GASCOIGNE_* (flags still filter)")
		fs.StringVar(&since, "since", "", "only the last `period`: 24h, 7d, 4w, 6mo")
		fs.BoolVar(&asJSON, "json", false, "print JSON instead of a table")
		fs.BoolVar(&oneline, "oneline", false, "print one line of totals, for a status line")
		fs.StringVar(&color, "color", "auto", "`mode`: auto (terminal, no NO_COLOR), always or never")
		fs.Float64Var(&rereads, "rereads", cost.AgentContext.Rereads, "later requests that read each tool result again, for the cost estimate (`n`; measured median 110)")
	}, func(l *ledger.Ledger, c *common, _ []string) error {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()
		if failures {
			if asJSON {
				return failuresJSON(ctx, l)
			}
			return printFailures(ctx, w, l)
		}
		id := c.identity()
		if all {
			id = mcptool.Identity{Session: c.session, Agent: c.agent, Model: c.model}
		}
		q := component.StatsQuery{By: by, Session: id.Session, Agent: id.Agent, Model: id.Model}
		if since != "" {
			d, err := parseSince(since)
			if err != nil {
				return err
			}
			q.Since = time.Now().Add(-d)
		}
		if oneline {
			return printOneline(ctx, os.Stdout, l, q, cost.Context{Rereads: rereads, Miss: cost.AgentContext.Miss})
		}
		p, err := newPainter(color)
		if err != nil {
			return err
		}
		if by == "" {
			return overview(ctx, l, q, asJSON, p, cost.Context{Rereads: rereads, Miss: cost.AgentContext.Miss})
		}
		stats, err := l.Stats(ctx, q)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(mcptool.Report(q, stats))
		}
		p.renderStats(os.Stdout, q, stats)
		return nil
	})
}

// printOneline prints the totals of q on one line, short enough for an
// agent's status line.
func printOneline(ctx context.Context, w io.Writer, l *ledger.Ledger, q component.StatsQuery, c cost.Context) error {
	o, err := mcptool.BuildOverview(ctx, l, q)
	if err != nil {
		return err
	}
	costs, err := mcptool.BuildCosts(ctx, l, q, c)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("gascoigne: %s tokens saved (%.0f%%)", human(o.Total.Saved), o.Total.SavedPct)
	if costs.InContext > 0 {
		line += fmt.Sprintf(" ~$%.2f", costs.InContext)
	}
	_, err = fmt.Fprintln(w, line)
	return err
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

func overview(ctx context.Context, l *ledger.Ledger, q component.StatsQuery, asJSON bool, p painter, c cost.Context) error {
	o, err := mcptool.BuildOverview(ctx, l, q)
	if err != nil {
		return err
	}
	if c != cost.AgentContext {
		if o.Cost, err = mcptool.BuildCosts(ctx, l, q, c); err != nil {
			return err
		}
	}
	failing, err := l.FailureList(ctx)
	if err != nil {
		return err
	}
	usage, err := l.UsageStats(ctx, q)
	if err != nil {
		return err
	}
	days, err := l.UsageDays(ctx, q)
	if err != nil {
		return err
	}
	if asJSON {
		return writeJSON(struct {
			mcptool.Overview
			FailingSources int                `json:"failing_sources"`
			APIUsage       []ledger.UsageStat `json:"api_usage"`
			APIDays        []ledger.UsageDay  `json:"api_days"`
		}{o, len(failing), usage, days})
	}
	p.renderOverview(os.Stdout, o, len(failing), usage, days)
	return nil
}

// parseSince reads "7d", "4w", "6mo" (30-day months) or a Go duration ("24h", "90m").
func parseSince(s string) (time.Duration, error) {
	day := 24 * time.Hour
	for _, u := range []struct {
		suffix string
		d      time.Duration
	}{{"mo", 30 * day}, {"d", day}, {"w", 7 * day}} {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, u.suffix)); strings.HasSuffix(s, u.suffix) && err == nil && n > 0 {
			return time.Duration(n) * u.d, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("-since: want 7d, 4w, 6mo or a duration like 24h, got %q", s)
	}
	return d, nil
}

// cmdReset clears savings stats. Filters come from flags only, never from
// GASCOIGNE_* env: deleting should say exactly what it deletes.
// cmdFlush deletes stored originals: the cache behind "gascoigne raw <id>".
func cmdFlush(ctx context.Context, args []string) error {
	var older string
	return withLedger(ctx, "flush", args, 0, func(fs *flag.FlagSet) {
		fs.StringVar(&older, "older", "", "only originals saved before this `period`: 24h, 7d, 4w, 6mo")
	}, func(l *ledger.Ledger, _ *common, _ []string) error {
		var cutoff time.Time
		if older != "" {
			d, err := parseSince(older)
			if err != nil {
				return err
			}
			cutoff = time.Now().Add(-d)
		}
		before, err := l.RawUsage(ctx)
		if err != nil {
			return err
		}
		n, err := l.FlushRaw(ctx, cutoff)
		if err != nil {
			return err
		}
		after, err := l.RawUsage(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("gascoigne: deleted %d stored originals (%s), %d left (%s)\n",
			n, humanBytes(before.Bytes-after.Bytes), after.Count, humanBytes(after.Bytes))
		return nil
	})
}

func humanBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

func cmdReset(ctx context.Context, args []string) error {
	var (
		all   bool
		since string
	)
	return withLedger(ctx, "reset", args, 0, func(fs *flag.FlagSet) {
		fs.BoolVar(&all, "all", false, "also delete stored originals and failure history (no filters)")
		fs.StringVar(&since, "since", "", "only runs of the last `period`: 24h, 7d, 4w, 6mo")
	}, func(l *ledger.Ledger, c *common, _ []string) error {
		q := component.StatsQuery{Session: c.session, Agent: c.agent, Model: c.model}
		if since != "" {
			d, err := parseSince(since)
			if err != nil {
				return err
			}
			q.Since = time.Now().Add(-d)
		}
		n, err := l.Reset(ctx, q, all)
		if err != nil {
			return err
		}
		fmt.Printf("gascoigne: deleted %d runs from %s", n, l.Path())
		if all {
			fmt.Print(", all stored originals and failure history")
		}
		fmt.Println()
		return nil
	})
}
