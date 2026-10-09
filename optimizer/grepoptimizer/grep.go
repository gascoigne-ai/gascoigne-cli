// Package grepoptimizer groups search results (grep, rg, git grep) by file:
// each path once, matches capped per file, long lines truncated.
package grepoptimizer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

// "path:12:text" (match) or "path-12-text" (context line).
var match = regexp.MustCompile(`^([^\s:][^:]*?)([:-])(\d+)([:-])(.*)$`)

type file struct {
	path  string
	lines []string
	total int
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		return compress.Join(o.group(compress.Lines(compress.StripANSI(b.Text)))), nil
	}, detect.Grep)
}

func (o *Optimizer) group(lines []string) []string {
	var files []*file
	var other []string
	var cur *file
	for _, l := range lines {
		m := match.FindStringSubmatch(l)
		if m == nil {
			if l != "--" {
				other = append(other, l)
			}
			continue
		}
		path, num, sep, text := strings.TrimPrefix(m[1], "./"), m[3], m[4], m[5]
		if cur == nil || cur.path != path {
			cur = &file{path: path}
			files = append(files, cur)
		}
		if sep == ":" {
			cur.total++
		}
		if len(cur.lines) < o.cfg.MaxPerFile {
			cur.lines = append(cur.lines, fmt.Sprintf("  %s%s %s", num, sep, compress.TruncateLine(strings.TrimSpace(text), o.cfg.MaxLineLen)))
		}
	}
	out := other
	for i, f := range files {
		if i == o.cfg.MaxFiles {
			out = append(out, compress.Omitted(len(files)-i, "files"))
			break
		}
		out = append(out, f.path)
		out = append(out, f.lines...)
		if hidden := f.total - countMatches(f.lines); hidden > 0 {
			out = append(out, "  "+compress.Omitted(hidden, "more matches"))
		}
	}
	return out
}

func countMatches(lines []string) int {
	n := 0
	for _, l := range lines {
		if num, _, ok := strings.Cut(strings.TrimSpace(l), " "); ok && strings.HasSuffix(num, ":") {
			n++
		}
	}
	return n
}
