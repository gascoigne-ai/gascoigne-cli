// Package truncateoptimizer is the last resort at the end of the chain: a
// block still over max_tokens keeps its head, its tail and the error lines
// in between; each cut leaves a marker, and the original stays retrievable.
package truncateoptimizer

import (
	"context"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
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
		if tokens.Estimate(b.Text) <= o.cfg.MaxTokens || o.skip(b.Type) {
			continue
		}
		p.Blocks[i].Text = o.cut(b.Text)
	}
	return nil
}

func (o *Optimizer) skip(typ string) bool {
	return slices.Contains(o.cfg.Skip, typ) || slices.Contains(o.cfg.Skip, detect.Family(typ))
}

func (o *Optimizer) cut(text string) string {
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
	out := slices.Clone(lines[:head])
	gap, errs := 0, 0
	for _, l := range lines[head:tail] {
		if errs < o.cfg.MaxErrors && errorRe.MatchString(l) {
			if gap > 0 {
				out = append(out, compress.Omitted(gap, ""))
			}
			out, gap = append(out, l), 0
			errs++
			continue
		}
		gap++
	}
	if gap > 0 {
		out = append(out, compress.Omitted(gap, ""))
	}
	return compress.Join(append(out, lines[tail:]...))
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
