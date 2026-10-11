// Package lsoptimizer is the fs family engine for listings: long ls output
// becomes names (directories first, files with human sizes, no permissions,
// owners or dates), tree output loses box drawing, path lists (find, fd)
// group by directory, du keeps the largest entries and df the real disks.
// Long runs of files fold to counts per extension.
package lsoptimizer

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

var entry = lazyre.New(`^([-dlcbps])([rwxsStT-]{9})[.+@]?\s+\d+\s+\S+\s+\S+\s+(\S+(?:,\s*\d+)?)\s+(?:\w{3}\s+\d{1,2}\s+(?:\d{1,2}:\d\d|\d{4})|\d{4}-\d\d-\d\d\s+\d\d:\d\d(?::\d\d(?:\.\d+)?)?(?:\s+[+-]\d{4})?)\s+(.+)$`)

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		lines := compress.Lines(compress.StripANSI(b.Text))
		switch b.Type {
		case detect.Tree:
			lines = o.tree(lines)
		case detect.Paths:
			lines = o.paths(lines)
		case detect.DU:
			lines = o.du(lines)
		case detect.DF:
			lines = o.df(lines)
		default:
			lines = o.rewrite(lines)
		}
		return compress.Join(lines), nil
	}, detect.LS, detect.Tree, detect.Paths, detect.DU, detect.DF)
}

type listing struct {
	dirs, files []string
	sizes       []string
}

func (o *Optimizer) rewrite(lines []string) []string {
	var out []string
	var cur listing
	parsed := false
	flush := func() {
		out = append(out, o.render(cur)...)
		cur = listing{}
	}
	for _, l := range lines {
		m := entry.FindStringSubmatch(l)
		switch {
		case m != nil:
			parsed = true
			cur.add(m[1], m[2], m[3], m[4])
		case strings.HasPrefix(l, "total "):
		case strings.HasSuffix(l, ":") || l == "":
			flush() // "dir:" header of ls -l a b, or blank between listings
			if l != "" {
				out = append(out, l)
			}
		default:
			out = append(out, l)
		}
	}
	flush()
	if !parsed {
		return lines // short format: already compact
	}
	return out
}

func (c *listing) add(kind, mode, size, name string) {
	switch kind {
	case "d":
		if name != "." && name != ".." {
			c.dirs = append(c.dirs, name+"/")
		}
		return
	case "l":
		target := ""
		if n, t, ok := strings.Cut(name, " -> "); ok {
			name, target = n, " -> "+t
		}
		c.files = append(c.files, name+"@"+target)
		c.sizes = append(c.sizes, "")
		return
	}
	if strings.ContainsAny(mode, "xs") && kind == "-" {
		name += "*"
	}
	c.files = append(c.files, name)
	c.sizes = append(c.sizes, human(size))
}

func (o *Optimizer) render(c listing) []string {
	var out []string
	if len(c.dirs) > 0 {
		out = append(out, wrap("dirs: ", c.dirs, o.cfg.Width)...)
	}
	if len(c.files) > 0 {
		files := c.files
		if len(files) <= o.cfg.SizesUpTo {
			files = make([]string, len(c.files))
			for i, f := range c.files {
				files[i] = strings.TrimSpace(f + " " + c.sizes[i])
			}
		}
		out = append(out, wrap(fmt.Sprintf("files (%d): ", len(files)), files, o.cfg.Width)...)
	}
	return out
}

// wrap joins items with two spaces into lines of at most width characters.
func wrap(prefix string, items []string, width int) []string {
	var out []string
	line := prefix
	for _, it := range items {
		if len(line) > len(prefix) && len(line)+2+len(it) > width {
			out = append(out, line)
			line = "  "
		}
		if line != prefix && line != "  " {
			line += "  "
		}
		line += it
	}
	return append(out, line)
}

func human(size string) string {
	n, err := strconv.ParseFloat(size, 64)
	if err != nil {
		return size // already human (ls -h) or a device number
	}
	for _, unit := range []string{"", "K", "M", "G"} {
		if n < 1024 {
			if unit == "" {
				return fmt.Sprintf("%.0f", n)
			}
			return strings.TrimSuffix(fmt.Sprintf("%.1f", n), ".0") + unit
		}
		n /= 1024
	}
	return fmt.Sprintf("%.1fT", n)
}
