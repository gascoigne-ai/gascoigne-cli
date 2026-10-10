package gitoptimizer

import (
	"fmt"
	"strings"
	"time"

	"github.com/lukaszb/gascoigne/lazyre"
)

// log rewrites default git log format to one line per commit:
// "<sha7> <date> <author> <subject>" plus refs and a body-size note. File
// lists (--stat, --name-only, --name-status) stay under their commit.
// Patches (git log -p) go through the diff compressor.
func (o *Optimizer) log(lines []string) []string {
	var out, patch []string
	var c commit
	emit := func() {
		if c.sha != "" {
			out = append(out, c.String())
			out = append(out, c.files...)
		}
		if len(patch) > 0 {
			out = append(out, o.diff(patch)...)
		}
		c, patch = commit{}, nil
	}
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "commit "):
			emit()
			c = parseCommitLine(l)
		case c.sha == "":
			out = append(out, l) // not default format (e.g. --oneline): keep
		case len(patch) > 0 || strings.HasPrefix(l, "diff --git "):
			patch = append(patch, l)
		case strings.HasPrefix(l, "Author: "):
			c.author, _, _ = strings.Cut(strings.TrimPrefix(l, "Author: "), " <")
		case strings.HasPrefix(l, "Date: "):
			c.date = shortDate(strings.TrimSpace(strings.TrimPrefix(l, "Date:")))
		case strings.HasPrefix(l, "Merge: "):
			c.merge = true
		case strings.HasPrefix(l, "    "):
			if c.subject == "" {
				c.subject = strings.TrimSpace(l)
			} else if strings.TrimSpace(l) != "" {
				c.body++
			}
		case strings.TrimSpace(l) != "":
			c.files = append(c.files, "  "+statLine(l))
		}
	}
	emit()
	return out
}

type commit struct {
	sha, refs, author, date, subject string
	body                             int
	merge                            bool
	files                            []string // --stat, --name-only, --name-status lines
}

var (
	// statCol is a --stat line: a padded path, then what changed.
	statCol = lazyre.New(`^\s*(.+?)\s+\|\s+(.+)$`)
	// statBar is the change count and a bar of + and - scaled to the terminal.
	statBar = lazyre.New(`^(\d+) ([+-]+)$`)
)

// statLine trims a file list line. A --stat bar is scaled, so only which
// signs it has carries information: "| 140 ++++++++" becomes "| 140 +".
func statLine(l string) string {
	m := statCol.FindStringSubmatch(l)
	if m == nil {
		return strings.TrimSpace(l)
	}
	change := m[2]
	if b := statBar.FindStringSubmatch(change); b != nil {
		change = b[1] + " "
		for _, s := range "+-" {
			if strings.ContainsRune(b[2], s) {
				change += string(s)
			}
		}
	}
	return m[1] + " | " + change
}

func parseCommitLine(l string) commit {
	f := strings.Fields(l)
	c := commit{sha: f[1][:min(7, len(f[1]))]}
	if i := strings.Index(l, "("); i > 0 {
		c.refs = l[i:]
	}
	return c
}

func (c commit) String() string {
	s := fmt.Sprintf("%s %s %s: %s", c.sha, c.date, c.author, c.subject)
	if c.refs != "" {
		s = c.sha + " " + c.refs + s[len(c.sha):]
	}
	if c.merge {
		s += " [merge]"
	}
	if c.body > 0 {
		s += fmt.Sprintf(" (+%d body lines)", c.body)
	}
	return s
}

func shortDate(s string) string {
	for _, layout := range []string{"Mon Jan 2 15:04:05 2006 -0700", time.RFC3339, "2006-01-02 15:04:05 -0700"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}
