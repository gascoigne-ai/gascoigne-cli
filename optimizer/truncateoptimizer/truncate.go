// Package truncateoptimizer is the last resort at the end of the chain: a
// block still over max_tokens keeps its head, its tail and the error lines
// in between; each cut leaves a marker, and the original stays retrievable.
package truncateoptimizer

import (
	"context"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

var errorRe = lazyre.New(`(?i)\b(error|errors|failed|failure|fatal|panic|exception|traceback)\b`)

// maxLine is the longest line kept whole, in bytes (minified code, one-line JSON).
const maxLine = 2000

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if o.skip(b.Type) || tokens.Estimate(b.Text) <= o.cfg.MaxTokens {
			continue
		}
		p.Blocks[i].Text = o.cut(b.Text, p.Attrs[payload.AttrQuery])
	}
	return nil
}

func (o *Optimizer) skip(typ string) bool {
	return detect.Matches(o.cfg.Skip, typ)
}

func (o *Optimizer) cut(text, query string) string {
	lines := compress.Lines(text)
	for i, l := range lines {
		if len(l) > maxLine {
			lines[i] = compress.TruncateLine(l, maxLine)
		}
	}
	head := fits(lines, o.cfg.HeadTokens)
	rev := slices.Clone(lines)
	slices.Reverse(rev)
	tail := len(lines) - fits(rev, o.cfg.TailTokens)
	if head >= tail {
		return compress.Join(lines)
	}
	keep := o.middle(lines[head:tail], sample.Terms(query))
	out := slices.Clone(lines[:head])
	gap := 0
	for i, l := range lines[head:tail] {
		if !keep[i] {
			gap++
			continue
		}
		if gap > 0 {
			out = append(out, compress.Omitted(gap, ""))
		}
		out, gap = append(out, l), 0
	}
	if gap > 0 {
		out = append(out, compress.Omitted(gap, ""))
	}
	return compress.Join(append(out, lines[tail:]...))
}

// mmrLambda weighs relevance against novelty in sample.MMR: equal.
const mmrLambda = 0.5

// middle marks the cut middle lines worth keeping: error lines with the
// ErrorContext lines after them (the got/want of a failure) and the lines
// that best match the query.
func (o *Optimizer) middle(mid, query []string) []bool {
	keep := make([]bool, len(mid))
	errs, after := 0, 0
	for i, l := range mid {
		switch {
		case errs < o.cfg.MaxErrors && errorRe.MatchString(l):
			keep[i], after = true, o.cfg.ErrorContext
			errs++
		case after > 0:
			keep[i] = true
			after--
		}
	}
	if len(query) == 0 || o.cfg.QueryLines == 0 {
		return keep
	}
	docs := make([][]string, len(mid))
	for i, l := range mid {
		docs[i] = sample.Terms(l)
	}
	scores := sample.BM25(docs, query)
	picks := sample.TopN(scores, o.cfg.QueryLines)
	if o.cfg.QueryDiverse {
		picks = sample.MMR(scores, docs, o.cfg.QueryLines, mmrLambda)
	}
	for _, i := range picks {
		keep[i] = true
	}
	return keep
}

// fits is how many leading lines stay within budget tokens.
func fits(lines []string, budget int) int {
	used := 0
	for i, l := range lines {
		if used += tokens.Estimate(l) + 1; used > budget {
			return i
		}
	}
	return len(lines)
}
