package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/component"
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

// newPainter decides on colors: never, always, or auto (a terminal and no NO_COLOR;
// CLICOLOR_FORCE set to anything but 0 counts as a terminal, for output piped through tee).
func newPainter(mode string) (painter, error) {
	switch mode {
	case "always":
		return painter{true}, nil
	case "never":
		return painter{false}, nil
	case "", "auto":
		fi, err := os.Stdout.Stat()
		tty := err == nil && fi.Mode()&os.ModeCharDevice != 0
		if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
			tty = true
		}
		return painter{tty && os.Getenv("NO_COLOR") == ""}, nil
	}
	return painter{}, fmt.Errorf("-color must be auto, always or never, got %q", mode)
}

func (p painter) paint(style, text string) string {
	if !p.color || style == "" || text == "" {
		return text
	}
	return "\x1b[" + style + "m" + text + "\x1b[0m"
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
func human(count int) string {
	f := float64(count)
	switch {
	case math.Abs(f) >= 1e6:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f/1e6), ".0") + "M"
	case math.Abs(f) >= 1e3:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f/1e3), ".0") + "k"
	}
	return fmt.Sprint(count)
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

func (p painter) table(writer io.Writer, header []string, rows [][]cell) {
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
		fmt.Fprintln(writer, strings.TrimRight(b.String(), " "))
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

func (p painter) section(writer io.Writer, title string) {
	fmt.Fprintf(writer, "\n%s\n", p.paint(bold+";"+cyan, strings.ToUpper(title)))
}

const barWidth = 20

// filterLabel describes active filters for titles.
func filterLabel(query component.StatsQuery) string {
	var parts []string
	for _, f := range []struct{ name, v string }{{"session", query.Session}, {"agent", query.Agent}, {"model", query.Model}} {
		if f.v != "" {
			parts = append(parts, f.name+" "+f.v)
		}
	}
	if !query.Since.IsZero() {
		parts = append(parts, "since "+query.Since.Format("2006-01-02 15:04"))
	}
	if len(parts) == 0 {
		return ""
	}
	return " · " + strings.Join(parts, " · ")
}

func zeroDim(count int) string {
	if count == 0 {
		return dim
	}
	return ""
}

// renderStats prints one grouping for terminals.
func (p painter) renderStats(writer io.Writer, query component.StatsQuery, stats []component.Stat, view gainView) {
	r := mcptool.Report(query, stats)
	title := "gascoigne savings by " + r.By + filterLabel(query)
	fmt.Fprintln(writer, p.paint(bold, title))
	if query.Chronological() {
		p.trendTable(writer, r.By, r.By, append(r.Groups, r.Total))
		return
	}
	shown, more := view.rows(r.Groups)
	p.savingsTable(writer, r.By, r.By, append(shown, r.Total), r.Total.Saved, r.By == component.ByOptimizer)
	p.more(writer, more, true)
}
