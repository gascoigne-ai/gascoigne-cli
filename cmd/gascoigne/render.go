package main

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// ANSI styles.
const (
	bold   = "1"
	dim    = "2"
	red    = "31"
	green  = "32"
	yellow = "33"
	cyan   = "36"
)

type painter struct{ color bool }

// newPainter decides on colors: never, always, or auto (a terminal and no NO_COLOR).
func newPainter(mode string) (painter, error) {
	switch mode {
	case "always":
		return painter{true}, nil
	case "never":
		return painter{false}, nil
	case "", "auto":
		fi, err := os.Stdout.Stat()
		tty := err == nil && fi.Mode()&os.ModeCharDevice != 0
		return painter{tty && os.Getenv("NO_COLOR") == ""}, nil
	}
	return painter{}, fmt.Errorf("-color must be auto, always or never, got %q", mode)
}

func (p painter) paint(style, s string) string {
	if !p.color || style == "" || s == "" {
		return s
	}
	return "\x1b[" + style + "m" + s + "\x1b[0m"
}

// savedStyle colors a saving: good, fair, poor, nothing.
func savedStyle(pct float64) string {
	switch {
	case pct >= 60:
		return green
	case pct >= 30:
		return yellow
	case pct > 0:
		return red
	}
	return dim
}

// human shortens token counts: 950, 1.2k, 47.4k, 1.3M.
func human(n int) string {
	f := float64(n)
	switch {
	case math.Abs(f) >= 1e6:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f/1e6), ".0") + "M"
	case math.Abs(f) >= 1e3:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f/1e3), ".0") + "k"
	}
	return fmt.Sprint(n)
}

var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// bar draws frac (0..1) as width cells of block characters, eighth precision.
func bar(frac float64, width int) string {
	frac = max(0, min(1, frac))
	units := int(math.Round(frac * float64(width*8)))
	s := strings.Repeat("█", units/8) + eighths[units%8]
	return s + strings.Repeat(" ", width-utf8.RuneCountInString(s))
}

func pct(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return 100 * float64(part) / float64(whole)
}

// cell is one table value; width is measured before painting.
type cell struct {
	text  string
	style string
	right bool
}

func (p painter) table(w io.Writer, header []string, rows [][]cell) {
	widths := make([]int, len(header))
	for i, h := range header {
		header[i] = strings.TrimSpace(h)
		widths[i] = utf8.RuneCountInString(header[i])
	}
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], utf8.RuneCountInString(c.text))
		}
	}
	line := func(cells []cell) {
		var b strings.Builder
		for i, c := range cells {
			pad := strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.text))
			text := p.paint(c.style, c.text)
			if c.right {
				text = pad + text
			} else if i < len(cells)-1 {
				text += pad
			}
			b.WriteString(" " + text + " ")
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
	head := make([]cell, len(header))
	for i, h := range header {
		head[i] = cell{text: h, style: dim, right: len(rows) > 0 && rows[0][i].right}
	}
	line(head)
	for _, r := range rows {
		line(r)
	}
}

func (p painter) section(w io.Writer, title string) {
	fmt.Fprintf(w, "\n%s\n", p.paint(bold+";"+cyan, strings.ToUpper(title)))
}

const barWidth = 20

// overviewData is what the overview sections draw from.
type overviewData struct {
	o       mcptool.Overview
	failing int
	usage   []ledger.UsageStat
	days    []ledger.UsageDay
	view    gainView
}

// overviewSections are the overview's blocks in print order; -only and -hide
// pick them by name.
var overviewSections = []struct {
	name string
	draw func(p painter, w io.Writer, d overviewData)
}{
	{"outcome", func(p painter, w io.Writer, d overviewData) { p.outcomeTable(w, d.o) }},
	{"trend", func(p painter, w io.Writer, d overviewData) { p.trendTable(w, d.trendTitle(), "day", d.o.Days) }},
	{"agent", func(p painter, w io.Writer, d overviewData) {
		d.savings(p, w, "agents", "agent", mcptool.Named(d.o.Agents), false)
	}},
	{"model", func(p painter, w io.Writer, d overviewData) {
		d.savings(p, w, "models", "model", mcptool.Named(d.o.Models), false)
	}},
	{"cost", func(p painter, w io.Writer, d overviewData) { p.costTable(w, d.o.Cost) }},
	{"usage", func(p painter, w io.Writer, d overviewData) { p.usageTable(w, d.usage) }},
	{"apidays", func(p painter, w io.Writer, d overviewData) { p.usageDaysTable(w, d.days) }},
	{"command", func(p painter, w io.Writer, d overviewData) {
		d.savings(p, w, "commands", "command", d.o.Commands, false)
	}},
	{"optimizer", func(p painter, w io.Writer, d overviewData) {
		d.savings(p, w, "optimizers", "optimizer", d.o.Optimizers, true)
	}},
	{"dialect", func(p painter, w io.Writer, d overviewData) {
		d.savings(p, w, "dialects", "dialect", d.o.Dialects, false)
	}},
	{"skipped", func(p painter, w io.Writer, d overviewData) {
		d.reasons(p, w, "skipped · not optimized on purpose", "filter", d.o.Skipped, yellow)
	}},
	{"failed", func(p painter, w io.Writer, d overviewData) {
		d.reasons(p, w, "failed · optimizer errors, original returned", "optimizer", d.o.Failed, red)
	}},
	{"unchanged", func(p painter, w io.Writer, d overviewData) {
		d.reasons(p, w, "unchanged · nothing to optimize", "", d.o.Unchanged, dim)
	}},
	{"failing", func(p painter, w io.Writer, d overviewData) {
		if d.failing > 0 {
			fmt.Fprintf(w, "\n%s %d sources bypass optimization after going bad · %s\n",
				p.paint(yellow, "⚠"), d.failing, p.paint(dim, "gascoigne gain -failures"))
		}
	}},
	{"estimates", func(p painter, w io.Writer, d overviewData) {
		if len(d.o.Estimates) == 0 {
			return
		}
		p.section(w, "estimates · how these numbers are made")
		for _, n := range d.o.Estimates {
			fmt.Fprintln(w, p.paint(dim, " "+n))
		}
	}},
}

func (d overviewData) trendTitle() string {
	if len(d.o.Days) == 7 {
		return "last 7 days"
	}
	return fmt.Sprintf("last %d days", len(d.o.Days))
}

// savings draws a savings table limited by the view; the share bars stay
// relative to all savings.
func (d overviewData) savings(p painter, w io.Writer, title, name string, rows []mcptool.StatsJSON, family bool) {
	shown, more := d.view.rows(rows)
	p.savingsTable(w, title+" · share of savings", name, shown, d.o.Total.Saved, family)
	p.more(w, more, len(shown) > 0)
}

func (d overviewData) reasons(p painter, w io.Writer, title, by string, rows []mcptool.StatsJSON, style string) {
	shown, more := d.view.rows(rows)
	p.reasonTable(w, title, by, shown, style)
	p.more(w, more, len(shown) > 0)
}

// more notes the rows a view limit left out.
func (p painter) more(w io.Writer, n int, drawn bool) {
	if n > 0 && drawn {
		fmt.Fprintln(w, p.paint(dim, fmt.Sprintf(" … %d more (-top 0 shows all)", n)))
	}
}

// renderOverview prints the gain overview for terminals.
func (p painter) renderOverview(w io.Writer, o mcptool.Overview, failing int, usage []ledger.UsageStat, days []ledger.UsageDay, v gainView) {
	p.overviewHeader(w, o)
	d := overviewData{o, failing, usage, days, v}
	for _, s := range overviewSections {
		if v.show(s.name) {
			s.draw(p, w, d)
		}
	}
}

func (p painter) overviewHeader(w io.Writer, o mcptool.Overview) {
	t := o.Total
	title := "gascoigne savings" + filterLabel(o.Filter)
	fmt.Fprintln(w, p.paint(bold, title))
	fmt.Fprintln(w, p.paint(dim, strings.Repeat("─", 64)))
	saved := pct(t.Saved, t.RawTokens)
	fmt.Fprintf(w, " %s %s  %s  %s\n",
		p.paint(dim, "saved"),
		p.paint(bold+";"+savedStyle(saved), human(t.Saved)+" tokens"),
		p.paint(bold+";"+savedStyle(saved), fmt.Sprintf("%.1f%%", saved)),
		p.paint(savedStyle(saved), bar(saved/100, 30)))
	skipped := 0
	for _, s := range o.Skipped {
		skipped += s.Runs
	}
	failed := p.paint(dim, fmt.Sprintf("%d failed", t.Failed))
	if t.Failed > 0 {
		failed = p.paint(red, fmt.Sprintf("%d failed", t.Failed))
	}
	fmt.Fprintf(w, " %s %s → %s   %s  %s, %s, %s\n",
		p.paint(dim, "tokens"), human(t.RawTokens), human(t.OutTokens),
		p.paint(dim, fmt.Sprintf("%d runs:", t.Runs)),
		p.paint(green, fmt.Sprintf("%d optimized", t.Optimized)),
		p.paint(yellow, fmt.Sprintf("%d skipped", skipped)), failed)
}

func (p painter) outcomeTable(w io.Writer, o mcptool.Overview) {
	if len(o.Outcomes) == 0 {
		return
	}
	p.section(w, "outcomes · share of input tokens")
	var rows [][]cell
	for _, r := range o.Outcomes {
		share := pct(r.RawTokens, o.Total.RawTokens)
		style := map[string]string{"optimized": green, "bypassed": yellow, "error": red, "unchanged": dim}[r.Group]
		rows = append(rows, []cell{
			{text: r.Group, style: style},
			{text: fmt.Sprint(r.Runs), right: true},
			{text: human(r.RawTokens), right: true},
			{text: human(r.OutTokens), right: true},
			{text: human(r.Saved), right: true, style: zeroDim(r.Saved)},
			{text: bar(share/100, barWidth) + fmt.Sprintf(" %5.1f%%", share), style: style},
		})
	}
	p.table(w, []string{"outcome", "runs", "in", "out", "saved", " share"}, rows)
}

// reasonTable lists runs that were not optimized with the facts behind it.
func (p painter) reasonTable(w io.Writer, title, by string, rows []mcptool.StatsJSON, style string) {
	if len(rows) == 0 {
		return
	}
	p.section(w, title)
	header := []string{"command", "runs", "tokens", " reason (latest)"}
	if by != "" {
		header = append([]string{by}, header...)
	}
	reasonStyle := dim
	if style == red {
		reasonStyle = red
	}
	var out [][]cell
	for _, r := range rows {
		row := []cell{
			{text: cmp.Or(r.Command, "-")},
			{text: fmt.Sprint(r.Runs), right: true},
			{text: human(r.RawTokens), right: true},
			{text: r.Reason, style: reasonStyle},
		}
		if by != "" {
			row = append([]cell{{text: r.Group, style: style}}, row...)
		}
		out = append(out, row)
	}
	p.table(w, header, out)
}

// trendTable shows periods oldest first with saved tokens as bars scaled to
// the best period.
func (p painter) trendTable(w io.Writer, title, name string, rows []mcptool.StatsJSON) {
	if len(rows) == 0 {
		return
	}
	p.section(w, title)
	best := 0
	for _, r := range rows {
		if r.Group != "total" {
			best = max(best, r.Saved)
		}
	}
	var out [][]cell
	for _, r := range rows {
		saved := pct(r.Saved, r.RawTokens)
		out = append(out, []cell{
			{text: r.Group},
			{text: fmt.Sprint(r.Runs), right: true},
			{text: human(r.RawTokens), right: true},
			{text: human(r.OutTokens), right: true},
			{text: human(r.Saved), right: true, style: zeroDim(r.Saved)},
			{text: fmt.Sprintf("%.1f%%", saved), right: true, style: savedStyle(saved)},
			{text: bar(float64(r.Saved)/float64(max(best, 1)), barWidth), style: cyan},
		})
	}
	p.table(w, []string{name, "runs", "in", "out", "saved", "saved %", " saved"}, out)
}

// filterLabel describes active filters for titles.
func filterLabel(q component.StatsQuery) string {
	var parts []string
	for _, f := range []struct{ name, v string }{{"session", q.Session}, {"agent", q.Agent}, {"model", q.Model}} {
		if f.v != "" {
			parts = append(parts, f.name+" "+f.v)
		}
	}
	if !q.Since.IsZero() {
		parts = append(parts, "since "+q.Since.Format("2006-01-02 15:04"))
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}

func zeroDim(n int) string {
	if n == 0 {
		return dim
	}
	return ""
}

// savingsTable lists groups with saved % and a bar of their share of all savings.
func (p painter) savingsTable(w io.Writer, title, name string, rows []mcptool.StatsJSON, totalSaved int, family bool) {
	if len(rows) == 0 {
		return
	}
	p.section(w, title)
	header := []string{name}
	if family {
		header = append(header, " family")
	}
	runs := "runs"
	if family || name == component.ByFamily || name == component.ByDialect {
		runs = "steps" // optimizer groupings count steps, not runs
	}
	header = append(header, runs, "in", "out", "saved", "saved %", " share")
	var out [][]cell
	for _, r := range rows {
		saved := pct(r.Saved, r.RawTokens)
		row := []cell{{text: cmp.Or(r.Group, "-")}}
		if r.Group == "total" {
			row[0].style = bold
		}
		if family {
			row = append(row, cell{text: cmp.Or(r.Family, "-"), style: dim})
		}
		row = append(row,
			cell{text: fmt.Sprint(r.Runs), right: true},
			cell{text: human(r.RawTokens), right: true},
			cell{text: human(r.OutTokens), right: true},
			cell{text: human(r.Saved), right: true, style: zeroDim(r.Saved)},
			cell{text: fmt.Sprintf("%.1f%%", saved), right: true, style: savedStyle(saved)},
			cell{text: bar(pct(r.Saved, totalSaved)/100, barWidth), style: cyan},
		)
		out = append(out, row)
	}
	p.table(w, header, out)
}

// renderStats prints one grouping for terminals.
func (p painter) renderStats(w io.Writer, q component.StatsQuery, stats []component.Stat, v gainView) {
	r := mcptool.Report(q, stats)
	title := "gascoigne savings by " + r.By + filterLabel(q)
	fmt.Fprintln(w, p.paint(bold, title))
	if q.Chronological() {
		p.trendTable(w, r.By, r.By, append(r.Groups, r.Total))
		return
	}
	shown, more := v.rows(r.Groups)
	p.savingsTable(w, r.By, r.By, append(shown, r.Total), r.Total.Saved, r.By == component.ByOptimizer)
	p.more(w, more, true)
}

// costTable prices the savings per model: once at first read, and with the
// rereads of every later request while tool results stay in context.
func (p painter) costTable(w io.Writer, c mcptool.Costs) {
	if len(c.Savings) == 0 && len(c.Unpriced) == 0 {
		return
	}
	p.section(w, "cost · what the saved tokens are worth")
	var rows [][]cell
	assumed := false
	for _, s := range c.Savings {
		name := s.Model
		if s.Assumed {
			name += "*"
			assumed = true
		}
		rows = append(rows, []cell{
			{text: name},
			{text: s.Tokenizer, style: dim},
			{text: human(s.ModelTokens), right: true},
			{text: dollars(s.FirstRead), right: true},
			{text: dollars(s.InContext), right: true, style: green},
		})
	}
	if len(c.Savings) > 1 {
		rows = append(rows, []cell{{text: "total", style: bold}, {}, {}, {text: dollars(c.FirstRead), right: true, style: bold},
			{text: dollars(c.InContext), right: true, style: bold + ";" + green}})
	}
	p.table(w, []string{"model", "tokenizer", "saved tok", "first read", "in context"}, rows)
	fmt.Fprintln(w, p.paint(dim, fmt.Sprintf(" in context: each tool result is read again by ~%.0f later requests (cache reads) until compaction · -rereads N", c.Rereads)))
	if assumed {
		fmt.Fprintln(w, p.paint(dim, " * no model recorded: the agent's default, set GASCOIGNE_MODEL or -model to record it"))
	}
	if len(c.Unpriced) > 0 {
		fmt.Fprintln(w, p.paint(yellow, " not priced: "+strings.Join(c.Unpriced, ", ")))
	}
}

// usageTable shows what the proxied API requests were billed, as the
// providers reported it, priced at list prices.
func (p painter) usageTable(w io.Writer, usage []ledger.UsageStat) {
	if len(usage) == 0 {
		return
	}
	p.section(w, "api usage · measured by the LLM proxy")
	var rows [][]cell
	var total float64
	unpriced := false
	for _, u := range usage {
		price := cell{text: "?", right: true, style: dim}
		if m, ok := cost.Lookup(u.Model); ok {
			usd := (float64(u.Input)*m.Input + float64(u.CacheRead)*m.CacheRead +
				float64(u.CacheWrite-u.CacheWrite1h)*m.CacheWrite + float64(u.CacheWrite1h)*oneHourWrite*m.Input + float64(u.Output)*m.Output) / 1e6
			price, total = cell{text: dollars(usd), right: true}, total+usd
		} else {
			unpriced = true
		}
		rows = append(rows, []cell{
			{text: cmp.Or(u.Agent, "-")}, {text: cmp.Or(u.Model, "-")},
			{text: strconv.Itoa(u.Requests), right: true},
			{text: human(u.MeanContext), right: true}, {text: human(u.PeakContext), right: true},
			{text: human(u.Input), right: true}, {text: human(u.CacheRead), right: true},
			{text: human(u.CacheWrite), right: true}, {text: human(u.Output), right: true}, price,
		})
	}
	if len(usage) > 1 {
		rows = append(rows, []cell{{text: "total", style: bold}, {}, {}, {}, {}, {}, {}, {}, {}, {text: dollars(total), right: true, style: bold}})
	}
	p.table(w, []string{"agent", "model", "requests", "context", "peak", "input", "cache read", "cache write", "output", "cost"}, rows)
	fmt.Fprintln(w, p.paint(dim, " context: tokens one request carries (input, cache read and write), mean and peak · cache writes at the 5-minute or 1-hour price as reported, where the provider reports the split"))
	if unpriced {
		fmt.Fprintln(w, p.paint(dim, " ? no known price for the model"))
	}
}

// usageDaysTable shows how many API requests a day the proxy saw and how big
// the conversation each one re-read was: the numbers that drive the bill.
func (p painter) usageDaysTable(w io.Writer, days []ledger.UsageDay) {
	if len(days) < 2 {
		return
	}
	p.section(w, "api requests by day · measured by the LLM proxy")
	var rows [][]cell
	for _, d := range days {
		rows = append(rows, []cell{
			{text: d.Day}, {text: strconv.Itoa(d.Requests), right: true},
			{text: human(d.MeanContext), right: true}, {text: human(d.PeakContext), right: true},
			{text: human(d.CacheRead), right: true}, {text: human(d.CacheWrite), right: true},
		})
	}
	p.table(w, []string{"day (UTC)", "requests", "context", "peak", "cache read", "cache write"}, rows)
}

func dollars(v float64) string {
	if v < 10 {
		return fmt.Sprintf("$%.2f", v)
	}
	return fmt.Sprintf("$%.0f", v)
}
