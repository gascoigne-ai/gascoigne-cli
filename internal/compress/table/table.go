// Package table parses tabular CLI output (whitespace-aligned like kubectl,
// docker ps, ps; boxed like psql, mysql, sqlite -box; markdown pipes) and
// renders it compactly: no padding or borders, constant columns hoisted,
// wide cells cut, long tables sampled keeping unhealthy rows.
package table

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Table is a parsed table with the lines around it.
type Table struct {
	Before []string // lines above the header (titles)
	Header []string
	Rows   [][]string
	After  []string // footers such as "(3 rows)"
}

var (
	borderRe = lazyre.New(`^\s*[+┌├└╞╘╔╠╚|│┃]?[-─━═+┬┼┴╤╪╧┐┤┘╡╛╗╣╝: ]*[-─━═]{3,}[-─━═+┬┼┴╤╪╧┐┤┘╡╛╗╣╝|│: ]*$`)
	cellSep  = lazyre.New(`\s*[|│┃]\s*`)
	gapRe    = lazyre.New(`\s{2,}`)
	spaceRe  = lazyre.New(`\s+`)
)

// Parse finds the table in lines; ok is false when there is none.
func Parse(lines []string) (Table, bool) {
	if t, ok := parseBoxed(lines); ok {
		return t, true
	}
	return parseAligned(lines)
}

// parseBoxed handles tables whose cells are separated by | or │.
func parseBoxed(lines []string) (Table, bool) {
	var t Table
	start := -1
	for i, l := range lines {
		if strings.ContainsAny(l, "|│┃") && !borderRe.MatchString(l) {
			start = i
			break
		}
	}
	if start < 0 {
		return t, false
	}
	t.Before = nonBorder(lines[:start])
	t.Header = cells(lines[start])
	if len(t.Header) < 2 {
		return t, false
	}
	i := start + 1
	for ; i < len(lines); i++ {
		l := lines[i]
		if borderRe.MatchString(l) {
			continue
		}
		if !strings.ContainsAny(l, "|│┃") {
			break
		}
		c := cells(l)
		if len(c) != len(t.Header) {
			break
		}
		t.Rows = append(t.Rows, c)
	}
	t.After = nonBorder(lines[i:])
	return t, len(t.Rows) > 0
}

func cells(l string) []string {
	l = strings.TrimSpace(l)
	l = strings.TrimFunc(l, func(r rune) bool { return r == '|' || r == '│' || r == '┃' })
	parts := cellSep.Split(strings.TrimSpace(l), -1)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func nonBorder(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !borderRe.MatchString(l) && strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// parseAligned handles whitespace-aligned columns with an upper-case header
// (kubectl get, docker ps, ps aux, systemctl, gcloud list).
func parseAligned(lines []string) (Table, bool) {
	if t, ok := parseRuled(lines); ok {
		return t, true
	}
	var t Table
	h := -1
	for i, l := range lines {
		if isHeader(l) {
			h = i
			break
		}
		if i > 5 {
			break
		}
	}
	if h < 0 || len(lines)-h < 3 {
		return t, false
	}
	t.Before = lines[:h]
	header := lines[h]
	byGap := gapRe.Split(strings.TrimSpace(header), -1)
	byWord := strings.Fields(header)
	body := lines[h+1:]
	end := len(body)
	for i, l := range body {
		if strings.TrimSpace(l) == "" {
			end = i
			break
		}
	}
	// Prefer columns separated by 2+ spaces (docker, kubectl): names may contain spaces.
	if r, ok := split(body[:end], len(byGap), func(l string) []string { return gapRe.Split(strings.TrimSpace(l), -1) }); ok && len(byGap) > 1 {
		t.Header, t.Rows = byGap, r
	} else if starts := columnStarts(header); len(starts) > 2 && len(starts) == len(byWord) || len(byGap) > 2 && len(starts) == len(byGap) {
		// Fixed-width columns with empty cells (docker ps PORTS, lsof TID).
		t.Header = slice(header, starts)
		for _, l := range body[:end] {
			t.Rows = append(t.Rows, slice(l, starts))
		}
	} else if r, ok := split(body[:end], len(byWord), func(l string) []string { return fieldsN(l, len(byWord)) }); ok {
		t.Header, t.Rows = byWord, r // single-space tables (ps, ss): last column takes the rest
	} else {
		return Table{}, false
	}
	t.After = body[end:]
	return t, len(t.Rows) >= 2
}

func split(lines []string, n int, f func(string) []string) ([][]string, bool) {
	var out [][]string
	for _, l := range lines {
		c := f(l)
		if len(c) != n {
			return nil, false
		}
		out = append(out, c)
	}
	return out, len(out) > 0
}

// fieldsN splits on whitespace into at most n fields.
func fieldsN(l string, n int) []string {
	f := strings.Fields(l)
	if len(f) <= n {
		return f
	}
	return append(f[:n-1:n-1], strings.Join(f[n-1:], " "))
}

// isHeader: 2+ tokens, mostly upper-case words (NAME, READY, %CPU, CONTAINER ID).
func isHeader(l string) bool {
	f := strings.Fields(l)
	if len(f) < 2 {
		return false
	}
	upper := 0
	for _, w := range f {
		if strings.IndexFunc(w, unicode.IsLetter) >= 0 && strings.ToUpper(w) == w {
			upper++
		}
	}
	return upper*4 >= len(f)*3
}

// Options tune Render.
type Options struct {
	MaxRows int // longer tables are sampled
	MaxCell int // longer cells are cut in the middle
	Rows    sample.Options
}

// Render prints the table compactly.
func Render(t Table, o Options) []string {
	out := append([]string{}, t.Before...)
	hdr, body, same := hoist(t.Header, t.Rows)
	if len(same) > 0 {
		out = append(out, "same in all rows: "+strings.Join(same, ", "))
	}
	rows := append([]string{strings.Join(hdr, cell)}, sampleRows(hdr, body, o)...)
	return append(append(out, align(rows)...), t.After...)
}

// cell separates cells of a rendered row until align lays them out.
const cell = "\x1f"

// align pads cells into columns two spaces apart, which real tokenizers
// read as one token per gap where " | " costs two or three. Rows with an
// empty cell or a cell holding a double space would be ambiguous: the
// table then keeps " | ". Lines without cells (omission markers) stay.
func align(lines []string) []string {
	var widths []int
	ambiguous := false
	for _, l := range lines {
		if !strings.Contains(l, cell) {
			continue
		}
		for i, c := range strings.Split(l, cell) {
			if c == "" || strings.Contains(c, "  ") {
				ambiguous = true
			}
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], utf8.RuneCountInString(c))
		}
	}
	out := make([]string, len(lines))
	for n, l := range lines {
		cs := strings.Split(l, cell)
		if len(cs) == 1 || ambiguous {
			out[n] = strings.Join(cs, " | ")
			continue
		}
		var b strings.Builder
		for i, c := range cs {
			b.WriteString(c)
			if i < len(cs)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c)+2))
			}
		}
		out[n] = b.String()
	}
	return out
}

// hoist moves columns with one value in every row out of the table.
func hoist(header []string, data [][]string) ([]string, [][]string, []string) {
	if len(data) < 3 {
		return header, data, nil
	}
	var keep []int
	var same []string
	for c := range header {
		v, constant := data[0][c], true
		for _, r := range data[1:] {
			if r[c] != v {
				constant = false
				break
			}
		}
		if constant {
			if v != "" {
				same = append(same, header[c]+"="+v)
			}
			continue
		}
		keep = append(keep, c)
	}
	pick := func(r []string) []string {
		out := make([]string, len(keep))
		for i, c := range keep {
			out[i] = r[c]
		}
		return out
	}
	newData := make([][]string, len(data))
	for i, r := range data {
		newData[i] = pick(r)
	}
	return pick(header), newData, same
}

// sampleRows keeps notable rows of long tables via sample.Sample.
func sampleRows(header []string, data [][]string, o Options) []string {
	line := func(r []string) string {
		cs := make([]string, len(r))
		for i, c := range r {
			cs[i] = compress.TruncateLine(c, o.MaxCell)
		}
		return strings.Join(cs, cell)
	}
	if o.MaxRows <= 0 || len(data) <= o.MaxRows {
		out := make([]string, len(data))
		for i, r := range data {
			out[i] = line(r)
		}
		return out
	}
	items := make([]any, len(data))
	for i, r := range data {
		m := map[string]any{"_i": i}
		for c, h := range header {
			m[strings.ToLower(h)] = value(r[c])
		}
		items[i] = m
	}
	opts := o.Rows
	opts.MaxItems = o.MaxRows
	var out []string
	for _, it := range sample.Sample(items, opts) {
		switch it := it.(type) {
		case string:
			out = append(out, it)
		case map[string]any:
			out = append(out, line(data[it["_i"].(int)]))
		}
	}
	return out
}

// value turns numeric cells into json.Number so anomaly signals see them.
func value(s string) any {
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return json.Number(s)
	}
	return s
}

// columnStarts are the byte offsets where header columns begin (columns are
// separated by 2+ spaces, or by one space when every header word is a column).
func columnStarts(header string) []int {
	var starts []int
	sep := gapRe
	if len(gapRe.Split(strings.TrimSpace(header), -1)) <= 2 {
		sep = spaceRe
	}
	pos := 0
	for _, part := range sep.Split(strings.TrimRight(header, " "), -1) {
		if part == "" {
			continue
		}
		i := strings.Index(header[pos:], part)
		starts = append(starts, pos+i)
		pos += i + len(part)
	}
	return starts
}

// slice cuts l at column starts, moving each cut left to a space so values
// that spill left (right-aligned numbers) stay whole.
func slice(l string, starts []int) []string {
	cuts := make([]int, len(starts))
	for i, st := range starts {
		c := min(st, len(l))
		for c > 0 && c < len(l) && l[c-1] != ' ' {
			c--
		}
		cuts[i] = c
	}
	out := make([]string, len(starts))
	for i := range starts {
		end := len(l)
		if i+1 < len(cuts) {
			end = max(cuts[i+1], cuts[i])
		}
		out[i] = strings.TrimSpace(l[cuts[i]:end])
	}
	return out
}

var ruleRe = lazyre.New(`^\s*-{2,}(\s+-{2,})+\s*$`)

// parseRuled handles a header underlined by dash groups (sqlite -column,
// SQL*Plus, many reports): the groups give exact column positions.
func parseRuled(lines []string) (Table, bool) {
	for i := 0; i+2 < len(lines) && i < 6; i++ {
		if !ruleRe.MatchString(lines[i+1]) {
			continue
		}
		var starts []int
		rule := lines[i+1]
		for j := 0; j < len(rule); j++ {
			if rule[j] == '-' && (j == 0 || rule[j-1] == ' ') {
				starts = append(starts, j)
			}
		}
		t := Table{Before: lines[:i], Header: slice(lines[i], starts)}
		k := i + 2
		for ; k < len(lines) && strings.TrimSpace(lines[k]) != ""; k++ {
			t.Rows = append(t.Rows, slice(lines[k], starts))
		}
		t.After = lines[k:]
		return t, len(t.Rows) > 0
	}
	return Table{}, false
}
