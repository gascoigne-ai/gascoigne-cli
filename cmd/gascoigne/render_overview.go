package main

import (
	"cmp"
	"fmt"
	"io"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

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
	draw func(pen painter, writer io.Writer, data overviewData)
}{
	{"outcome", func(pen painter, writer io.Writer, data overviewData) { pen.outcomeTable(writer, data.o) }},
	{"trend", func(pen painter, writer io.Writer, data overviewData) {
		pen.trendTable(writer, data.trendTitle(), "day", data.o.Days)
	}},
	{"agent", func(pen painter, writer io.Writer, data overviewData) {
		data.savings(pen, writer, "agents", "agent", mcptool.Named(data.o.Agents), false)
	}},
	{"model", func(pen painter, writer io.Writer, data overviewData) {
		data.savings(pen, writer, "models", "model", mcptool.Named(data.o.Models), false)
	}},
	{"cost", func(pen painter, writer io.Writer, data overviewData) { pen.costTable(writer, data.o.Cost) }},
	{"usage", func(pen painter, writer io.Writer, data overviewData) { pen.usageTable(writer, data.usage) }},
	{"apidays", func(pen painter, writer io.Writer, data overviewData) { pen.usageDaysTable(writer, data.days) }},
	{"command", func(pen painter, writer io.Writer, data overviewData) {
		data.savings(pen, writer, "commands", "command", data.o.Commands, false)
	}},
	{"optimizer", func(pen painter, writer io.Writer, data overviewData) {
		data.savings(pen, writer, "optimizers", "optimizer", data.o.Optimizers, true)
	}},
	{"dialect", func(pen painter, writer io.Writer, data overviewData) {
		data.savings(pen, writer, "dialects", "dialect", data.o.Dialects, false)
	}},
	{"skipped", func(pen painter, writer io.Writer, data overviewData) {
		data.reasons(pen, writer, "skipped · not optimized on purpose", "filter", data.o.Skipped, yellow)
	}},
	{"failed", func(pen painter, writer io.Writer, data overviewData) {
		data.reasons(pen, writer, "failed · optimizer errors, original returned", "optimizer", data.o.Failed, red)
	}},
	{"unchanged", func(pen painter, writer io.Writer, data overviewData) {
		data.reasons(pen, writer, "unchanged · nothing to optimize", "", data.o.Unchanged, dim)
	}},
	{"failing", func(pen painter, writer io.Writer, data overviewData) {
		if data.failing > 0 {
			fmt.Fprintf(writer, "\n%s %d sources bypass optimization after going bad · %s\n",
				pen.paint(yellow, "⚠"), data.failing, pen.paint(dim, "gascoigne gain -failures"))
		}
	}},
	{"estimates", func(pen painter, writer io.Writer, data overviewData) {
		if len(data.o.Estimates) == 0 {
			return
		}
		pen.section(writer, "estimates · how these numbers are made")
		for _, n := range data.o.Estimates {
			fmt.Fprintln(writer, pen.paint(dim, " "+n))
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
func (d overviewData) savings(pen painter, writer io.Writer, title, name string, rows []mcptool.StatsJSON, family bool) {
	shown, more := d.view.rows(rows)
	pen.savingsTable(writer, title+" · share of savings", name, shown, d.o.Total.Saved, family)
	pen.more(writer, more, len(shown) > 0)
}

func (d overviewData) reasons(pen painter, writer io.Writer, title, by string, rows []mcptool.StatsJSON, style string) {
	shown, more := d.view.rows(rows)
	pen.reasonTable(writer, title, by, shown, style)
	pen.more(writer, more, len(shown) > 0)
}

// more notes the rows a view limit left out.
func (p painter) more(writer io.Writer, count int, drawn bool) {
	if count > 0 && drawn {
		fmt.Fprintln(writer, p.paint(dim, fmt.Sprintf(" … %d more (-top 0 shows all)", count)))
	}
}

// renderOverview prints the gain overview for terminals.
func (p painter) renderOverview(writer io.Writer, overview mcptool.Overview, failing int, usage []ledger.UsageStat, days []ledger.UsageDay, view gainView) {
	p.overviewHeader(writer, overview)
	d := overviewData{overview, failing, usage, days, view}
	for _, s := range overviewSections {
		if view.show(s.name) {
			s.draw(p, writer, d)
		}
	}
}

func (p painter) overviewHeader(writer io.Writer, overview mcptool.Overview) {
	t := overview.Total
	title := "gascoigne savings" + filterLabel(overview.Filter)
	fmt.Fprintln(writer, p.paint(bold, title))
	fmt.Fprintln(writer, p.paint(dim, strings.Repeat("─", 64)))
	saved := pct(t.Saved, t.RawTokens)
	fmt.Fprintf(writer, " %s %s  %s  %s\n",
		p.paint(dim, "saved"),
		p.paint(bold+";"+savedStyle(saved), human(t.Saved)+" tokens"),
		p.paint(bold+";"+savedStyle(saved), fmt.Sprintf("%.1f%%", saved)),
		p.paint(savedStyle(saved), bar(saved/100, 30)))
	skipped := 0
	for _, s := range overview.Skipped {
		skipped += s.Runs
	}
	failed := p.paint(dim, fmt.Sprintf("%d failed", t.Failed))
	if t.Failed > 0 {
		failed = p.paint(red, fmt.Sprintf("%d failed", t.Failed))
	}
	fmt.Fprintf(writer, " %s %s → %s   %s  %s, %s, %s\n",
		p.paint(dim, "tokens"), human(t.RawTokens), human(t.OutTokens),
		p.paint(dim, fmt.Sprintf("%d runs:", t.Runs)),
		p.paint(green, fmt.Sprintf("%d optimized", t.Optimized)),
		p.paint(yellow, fmt.Sprintf("%d skipped", skipped)), failed)
	if c := overview.Cost.InContext; c > 0 {
		fmt.Fprintf(writer, " %s ~%s at list prices, rereads from the prompt cache included\n", p.paint(dim, "worth"), dollars(c))
	}
}

func (p painter) outcomeTable(writer io.Writer, overview mcptool.Overview) {
	if len(overview.Outcomes) == 0 {
		return
	}
	p.section(writer, "outcomes · share of input tokens")
	var rows [][]cell
	for _, r := range overview.Outcomes {
		share := pct(r.RawTokens, overview.Total.RawTokens)
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
	p.table(writer, []string{"outcome", "runs", "in", "out", "saved", " share"}, rows)
}

// reasonTable lists runs that were not optimized with the facts behind it.
func (p painter) reasonTable(writer io.Writer, title, by string, rows []mcptool.StatsJSON, style string) {
	if len(rows) == 0 {
		return
	}
	p.section(writer, title)
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
	p.table(writer, header, out)
}

// trendTable shows periods oldest first with saved tokens as bars scaled to
// the best period.
func (p painter) trendTable(writer io.Writer, title, name string, rows []mcptool.StatsJSON) {
	if len(rows) == 0 {
		return
	}
	p.section(writer, title)
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
	p.table(writer, []string{name, "runs", "in", "out", "saved", "saved %", " saved"}, out)
}

// savingsTable lists groups with saved % and a bar of their share of all savings.
func (p painter) savingsTable(writer io.Writer, title, name string, rows []mcptool.StatsJSON, totalSaved int, family bool) {
	if len(rows) == 0 {
		return
	}
	p.section(writer, title)
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
	p.table(writer, header, out)
}
