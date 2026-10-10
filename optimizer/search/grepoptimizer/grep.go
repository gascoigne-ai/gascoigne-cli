// Package grepoptimizer groups search results (grep, rg, git grep) by file:
// each path once, identical lines merged, distinct lines capped per file,
// long lines truncated.
package grepoptimizer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

var (
	// "path:12:text" (match) or "path-12-text" (context line).
	numbered = lazyre.New(`^([^\s:][^:]*?)([:-])(\d+)([:-])(.*)$`)
	// "path:text": rg and grep -r without -n, rg's default when not on a terminal.
	plain = lazyre.New(`^([^\s:]+):(.*)$`)
	ext   = lazyre.New(`\.[A-Za-z0-9]{1,6}$`)
)

// hit is one distinct line of a file; identical lines merge into it.
type hit struct {
	nums  []string
	sep   string
	text  string
	count int
}

type file struct {
	path   string
	hits   []*hit
	byText map[string]*hit
	total  int // matches, without context lines
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		return compress.Join(o.group(compress.Lines(compress.StripANSI(b.Text)))), nil
	}, detect.Grep, detect.RG, detect.AG, detect.GitGrep)
}

func (o *Optimizer) group(lines []string) []string {
	parse := parseNumbered
	if !slices.ContainsFunc(lines, numbered.MatchString) && plainBlock(lines) {
		parse = parsePlain
	}
	var files []*file
	byPath := map[string]*file{}
	var other []string
	for _, l := range lines {
		path, num, sep, text, ok := parse(l)
		if !ok {
			if l != "--" {
				other = append(other, l)
			}
			continue
		}
		f := byPath[path]
		if f == nil {
			f = &file{path: path, byText: map[string]*hit{}}
			byPath[path] = f
			files = append(files, f)
		}
		o.add(f, num, sep, text)
	}
	out := other
	for i, f := range files {
		if i == o.cfg.MaxFiles {
			out = append(out, compress.Omitted(len(files)-i, "files"))
			break
		}
		out = append(out, f.path)
		out = append(out, f.render()...)
	}
	return out
}

func (o *Optimizer) add(f *file, num, sep, text string) {
	if sep == ":" {
		f.total++
	}
	text = compress.TruncateLine(strings.TrimSpace(text), o.cfg.MaxLineLen)
	if h := f.byText[sep+text]; h != nil {
		h.count++
		if num != "" {
			h.nums = append(h.nums, num)
		}
		return
	}
	if len(f.hits) == o.cfg.MaxPerFile {
		return
	}
	h := &hit{sep: sep, text: text, count: 1}
	if num != "" {
		h.nums = []string{num}
	}
	f.hits = append(f.hits, h)
	f.byText[sep+text] = h
}

func (f *file) render() []string {
	var out []string
	shown := 0
	for _, h := range f.hits {
		switch {
		case len(h.nums) > 0:
			out = append(out, fmt.Sprintf("  %s%s %s", strings.Join(h.nums, ","), h.sep, h.text))
		case h.count > 1:
			out = append(out, fmt.Sprintf("  %d× %s", h.count, h.text))
		default:
			out = append(out, "  "+h.text)
		}
		if h.sep == ":" {
			shown += h.count
		}
	}
	if hidden := f.total - shown; hidden > 0 {
		out = append(out, "  "+compress.Omitted(hidden, "more matches"))
	}
	return out
}

func parseNumbered(l string) (path, num, sep, text string, ok bool) {
	m := numbered.FindStringSubmatch(l)
	if m == nil {
		return "", "", "", "", false
	}
	return strings.TrimPrefix(m[1], "./"), m[3], m[4], m[5], true
}

func parsePlain(l string) (path, num, sep, text string, ok bool) {
	m := plain.FindStringSubmatch(l)
	if m == nil || !pathLike(m[1]) {
		return "", "", "", "", false
	}
	return strings.TrimPrefix(m[1], "./"), "", ":", m[2], true
}

// plainBlock reports search output without line numbers: nearly every line
// starts with a file path and some path repeats. Single-file grep output
// ("key.name: 1") rarely repeats a prefix, so it stays as it is.
func plainBlock(lines []string) bool {
	seen := map[string]bool{}
	parsed, candidates, repeats := 0, 0, false
	for _, l := range lines {
		if l == "" || l == "--" {
			continue
		}
		candidates++
		if path, _, _, _, ok := parsePlain(l); ok {
			parsed++
			repeats = repeats || seen[path]
			seen[path] = true
		}
	}
	return repeats && parsed*10 >= candidates*9
}

func pathLike(s string) bool { return strings.Contains(s, "/") || ext.MatchString(s) }
