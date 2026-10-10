// Package pathsoptimizer replaces long path prefixes that repeat across
// output (stack traces, diagnostics, matches) with short ids defined once:
//
//	[paths: §1=/home/runner/go/pkg/mod/github.com/acme/api@v1.4.2]
//	§1/router.go:42
//
// File names stay visible; the legend gives every full path back.
package pathsoptimizer

import (
	"cmp"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/tokens"
)

const legendPrefix = "[paths: "

type Optimizer struct {
	component.Nop
	cfg *Config
}

// pathRe finds paths of at least three parts, starting after a delimiter.
var pathRe = regexp.MustCompile(`(?:^|[\s"'(=\[<,])((?:[A-Za-z]:)?[\w.~@+-]*(?:/[\w.@+-]+){2,})`)

type span struct {
	block, start, end int // the path in p.Blocks[block].Text
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	var spans []span
	for i, b := range p.Blocks {
		if !slices.Contains(o.cfg.Families, detect.Family(b.Type)) || strings.Contains(b.Text, legendPrefix) {
			continue
		}
		for _, m := range pathRe.FindAllStringSubmatchIndex(b.Text, -1) {
			spans = append(spans, span{i, m[2], m[3]})
		}
	}
	aliases := o.pick(p, spans)
	if aliases == nil {
		return nil
	}
	before := tokens.Estimate(p.Text())
	out := p.Clone()
	o.apply(out, spans, aliases)
	if before-tokens.Estimate(out.Text()) < o.cfg.MinSaving {
		return nil
	}
	p.Blocks = out.Blocks
	return nil
}

// pick chooses prefixes greedily by bytes saved, legend entries included.
func (o *Optimizer) pick(p *payload.Payload, spans []span) []string {
	var aliases []string
	taken := make([]bool, len(spans))
	for len(aliases) < o.cfg.MaxAliases {
		alias := id(len(aliases))
		best, gain := "", 0
		for prefix, n := range o.counts(p, spans, taken) {
			g := n*(len(prefix)-len(alias)) - len(alias) - len(prefix) - 2
			if g > gain || g == gain && prefix < best {
				best, gain = prefix, g
			}
		}
		if best == "" {
			break
		}
		aliases = append(aliases, best)
		for i, s := range spans {
			if strings.HasPrefix(text(p, s), best+"/") {
				taken[i] = true
			}
		}
	}
	return aliases
}

// counts is how many free paths start with each candidate prefix (cut at a
// slash, at least MinPrefix long, leaving the file name).
func (o *Optimizer) counts(p *payload.Payload, spans []span, taken []bool) map[string]int {
	counts := map[string]int{}
	for i, s := range spans {
		if taken[i] {
			continue
		}
		path := text(p, s)
		for j := range len(path) {
			if path[j] == '/' && j >= o.cfg.MinPrefix && j < strings.LastIndexByte(path, '/')+1 {
				counts[path[:j]]++
			}
		}
	}
	for prefix, n := range counts {
		if n < 2 {
			delete(counts, prefix)
		}
	}
	return counts
}

func (o *Optimizer) apply(p *payload.Payload, spans []span, aliases []string) {
	// Replace from the end so earlier offsets stay valid.
	slices.SortFunc(spans, func(a, b span) int { return cmp.Or(b.block-a.block, b.start-a.start) })
	for _, s := range spans {
		t := p.Blocks[s.block].Text
		path := t[s.start:s.end]
		for i, prefix := range aliases {
			if strings.HasPrefix(path, prefix+"/") {
				p.Blocks[s.block].Text = t[:s.start] + id(i) + path[len(prefix):] + t[s.end:]
				break
			}
		}
	}
	legend := make([]string, len(aliases))
	for i, a := range aliases {
		legend[i] = id(i) + "=" + a
	}
	first := slices.IndexFunc(p.Blocks, func(b payload.Block) bool {
		return slices.Contains(o.cfg.Families, detect.Family(b.Type))
	})
	p.Blocks[first].Text = legendPrefix + strings.Join(legend, " ") + "]\n" + p.Blocks[first].Text
}

func text(p *payload.Payload, s span) string { return p.Blocks[s.block].Text[s.start:s.end] }

func id(i int) string { return fmt.Sprintf("§%d", i+1) }
