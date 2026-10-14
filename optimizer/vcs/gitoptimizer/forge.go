package gitoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

var blameLine = lazyre.New(`^\^?([0-9a-f]{7,40}) (?:\S+ )?\((.+?) (\d{4}-\d\d-\d\d) \d\d:\d\d:\d\d [+-]\d{4} +(\d+)\) ?(.*)$`)

// blame prints one header per run of lines from the same commit instead of
// repeating hash, author and time on every line.
func blame(lines []string) []string {
	var out []string
	prev := ""
	for _, l := range lines {
		m := blameLine.FindStringSubmatch(l)
		if m == nil {
			out = append(out, l)
			continue
		}
		if head := m[1][:8] + " " + m[2] + " " + m[3]; head != prev {
			out = append(out, head)
			prev = head
		}
		out = append(out, fmt.Sprintf("%5s %s", m[4], m[5]))
	}
	return out
}

// firstField is the first word of s, or "" when s is blank.
func firstField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// branch drops remote branches mirrored by a local one and caps the rest.
func (o *Optimizer) branch(lines []string) []string {
	local := map[string]bool{}
	for _, l := range lines {
		if name := strings.TrimSpace(strings.TrimPrefix(l, "*")); !strings.HasPrefix(name, "remotes/") {
			local[firstField(name)] = true
		}
	}
	var out, remote []string
	mirrored := 0
	for _, l := range lines {
		name := strings.TrimSpace(strings.TrimPrefix(l, "*"))
		r, ok := strings.CutPrefix(name, "remotes/")
		switch {
		case !ok:
			out = append(out, l)
		case local[firstField(r[strings.Index(r, "/")+1:])] && !strings.Contains(r, "->"):
			mirrored++
		default:
			remote = append(remote, l)
		}
	}
	out = append(out, compress.HeadTail(remote, o.cfg.MaxFileLines/10, o.cfg.MaxFileLines/20)...)
	if mirrored > 0 {
		out = append(out, compress.Omitted(mirrored, "remote branches mirrored locally"))
	}
	return out
}

// Columns gh prints, tab separated, when its output is not a terminal.
var ghColumns = map[int][]string{
	5: {"ID", "TITLE", "BRANCH", "STATE", "CREATED"},
	9: {"STATUS", "CONCLUSION", "TITLE", "WORKFLOW", "BRANCH", "EVENT", "ID", "ELAPSED", "AGE"},
}

// ghList compacts gh list output, tab separated or aligned, as a table.
func (o *Optimizer) ghList(lines []string, query []string) []string {
	opts := table.Options{MaxRows: 30, MaxCell: 80, Rows: sample.Options{HeadShare: 0.5, TailShare: 0.15, RelevanceK: 3, Query: query}}
	if len(lines) > 0 && strings.Contains(lines[0], "\t") {
		var t table.Table
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				t.Rows = append(t.Rows, strings.Split(l, "\t"))
			}
		}
		n := len(t.Rows[0])
		t.Header = ghColumns[n]
		for i := len(t.Header); i < n; i++ {
			t.Header = append(t.Header, fmt.Sprintf("C%d", i+1))
		}
		return table.Render(t, opts)
	}
	if t, ok := table.Parse(lines); ok {
		return table.Render(t, opts)
	}
	return lines
}

var (
	emptyField  = lazyre.New(`^[a-z-]+:\t*$`)
	htmlComment = lazyre.New(`(?s)<!--.*?-->`)
)

// ghView drops empty metadata fields and template comments.
func ghView(text string) []string {
	var out []string
	for _, l := range compress.Lines(htmlComment.ReplaceAllString(text, "")) {
		if !emptyField.MatchString(l) {
			out = append(out, strings.Replace(l, ":\t", ": ", 1))
		}
	}
	return compress.SqueezeBlank(out)
}
