package main

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/mcptool"
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

// renderOverview prints the gain overview for terminals.
func (p painter) renderOverview(w io.Writer, o mcptool.Overview, failing int) {
	t := o.Total
	title := "gascoigne savings"
	if o.Session != "" {
		title += " · session " + o.Session
	}
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

	if len(o.Outcomes) > 0 {
		p.section(w, "outcomes · share of input tokens")
		var rows [][]cell
		for _, r := range o.Outcomes {
			share := pct(r.RawTokens, t.RawTokens)
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
	p.savingsTable(w, "commands · share of savings", "command", o.Commands, t.Saved, false)
	p.savingsTable(w, "optimizers · share of savings", "optimizer", o.Optimizers, t.Saved, true)

	p.reasonTable(w, "skipped · not optimized on purpose", "filter", o.Skipped, yellow)
	p.reasonTable(w, "failed · optimizer errors, original returned", "optimizer", o.Failed, red)
	p.reasonTable(w, "unchanged · nothing to optimize", "", o.Unchanged, dim)
	if failing > 0 {
		fmt.Fprintf(w, "\n%s %d sources bypass optimization after going bad · %s\n",
			p.paint(yellow, "⚠"), failing, p.paint(dim, "gascoigne gain -failures"))
	}
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
	if family || name == component.ByFamily {
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
func (p painter) renderStats(w io.Writer, q component.StatsQuery, stats []component.Stat) {
	r := mcptool.Report(q, stats)
	title := "gascoigne savings by " + r.By
	if q.Session != "" {
		title += " · session " + q.Session
	}
	fmt.Fprintln(w, p.paint(bold, title))
	p.savingsTable(w, r.By, r.By, append(r.Groups, r.Total), r.Total.Saved, r.By == component.ByOptimizer)
}
