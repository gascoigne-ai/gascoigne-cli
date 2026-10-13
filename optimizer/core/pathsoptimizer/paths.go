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
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress/legend"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

// pathRe finds paths of at least three parts, starting after a delimiter.
var pathRe = lazyre.New(`(?:^|[\s"'(=\[<,])((?:[A-Za-z]:)?[\w.~@+-]*(?:/[\w.@+-]+){2,})`)

type span struct {
	block, start, end int // the path in p.Blocks[block].Text
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	var spans []span
	for i, b := range p.Blocks {
		if !slices.Contains(o.cfg.Families, detect.Family(b.Type)) || legend.Has(b.Text) {
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
		alias := legend.ID(legend.Paths, len(aliases))
		best, gain := "", 0
		for prefix, n := range o.counts(p, spans, taken) {
			g := legend.Worth(n, len(prefix), len(alias))
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
				p.Blocks[s.block].Text = t[:s.start] + legend.ID(legend.Paths, i) + path[len(prefix):] + t[s.end:]
				break
			}
		}
	}
	defs := make([]legend.Def, len(aliases))
	for i, a := range aliases {
		defs[i] = legend.Def{Id: legend.ID(legend.Paths, i), Value: a}
	}
	first := slices.IndexFunc(p.Blocks, func(b payload.Block) bool {
		return slices.Contains(o.cfg.Families, detect.Family(b.Type))
	})
	p.Blocks[first].Text = legend.Render(legend.Paths, defs) + "\n" + p.Blocks[first].Text
}

func text(p *payload.Payload, s span) string { return p.Blocks[s.block].Text[s.start:s.end] }
