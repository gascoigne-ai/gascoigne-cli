// Package rerunoptimizer folds what a re-run repeats: agents re-run tests and
// builds after each edit, and a failure unchanged since the last run was
// already shown. Runs of lines an earlier output of the same command in the
// same session showed (within max_age) become one pointer line. The output
// depends on the ledger's history, not only on the input; without a session
// or a history it changes nothing.
package rerunoptimizer

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/legend"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// maxAnchors caps the places tried per line, so common lines (blank, "}",
// "ok") cannot make matching quadratic.
const maxAnchors = 16

type Optimizer struct {
	component.Nop
	cfg     *Config
	history component.OutputHistory
	now     func() time.Time
}

func (o *Optimizer) Start(_ context.Context, host component.Host) error {
	o.now = time.Now
	if o.cfg.Ledger == "" {
		return nil
	}
	l, err := mcptool.Ledger(host, o.cfg.Ledger)
	if err != nil {
		return fmt.Errorf("rerun ledger: %w", err)
	}
	o.history, _ = l.(component.OutputHistory)
	return nil
}

func (o *Optimizer) Optimize(ctx context.Context, p *payload.Payload) error {
	session := p.Attrs[payload.AttrSession]
	if o.history == nil || session == "" || !slices.ContainsFunc(p.Blocks, o.eligible) {
		return nil
	}
	outs, err := o.history.RecentOutputs(ctx, session, p.Key(), o.now().Add(-o.cfg.MaxAge), o.cfg.Runs)
	if err != nil || len(outs) == 0 {
		return err
	}
	ref := newReference(outs)
	folded := 0
	for i, b := range p.Blocks {
		if !o.eligible(b) {
			continue
		}
		text, n := ref.fold(b.Text, o.cfg.MinLines, o.cfg.MinChars)
		p.Blocks[i].Text = text
		folded += n
	}
	if folded == 0 {
		p.Set(payload.AttrSkip, "rerun: nothing shown in the last runs")
	}
	return nil
}

func (o *Optimizer) eligible(b payload.Block) bool {
	return slices.Contains(o.cfg.Families, detect.Family(b.Type))
}

// reference holds the lines of earlier outputs and where each starts.
type reference struct {
	runs    [][]string
	anchors map[string][][2]int // line → (run, index) places, capped
}

func newReference(outs []string) *reference {
	r := &reference{anchors: map[string][][2]int{}}
	for i, out := range outs {
		lines := compress.Lines(out)
		r.runs = append(r.runs, lines)
		for j, l := range lines {
			if !anchors(l) {
				continue
			}
			if s := r.anchors[l]; len(s) < maxAnchors {
				r.anchors[l] = append(s, [2]int{i, j})
			}
		}
	}
	return r
}

// fold replaces runs of text shown earlier by one pointer line each and
// returns the text and how many lines it folded.
func (r *reference) fold(text string, minLines, minChars int) (string, int) {
	lines := compress.Lines(text)
	var out []string
	folded := 0
	for i := 0; i < len(lines); {
		n := r.longest(lines, i)
		if n >= minLines && size(lines[i:i+n]) >= minChars {
			out = append(out, pointer(lines[i:i+n]))
			folded += n
			i += n
			continue
		}
		out = append(out, lines[i])
		i++
	}
	if folded == 0 {
		return text, 0
	}
	return compress.Join(out), folded
}

// longest is the longest run from lines[i] an earlier output shows verbatim.
func (r *reference) longest(lines []string, i int) int {
	best := 0
	for _, at := range r.anchors[lines[i]] {
		ref := r.runs[at[0]]
		k := 0
		for i+k < len(lines) && at[1]+k < len(ref) && lines[i+k] == ref[at[1]+k] && !legend.Is(lines[i+k]) {
			k++
		}
		best = max(best, k)
	}
	return best
}

// pointer names the folded run by its first line, so the agent finds it in
// the earlier output.
func pointer(run []string) string {
	first := strings.TrimSpace(run[0])
	if r := []rune(first); len(r) > 50 {
		first = string(r[:49]) + "…"
	}
	return fmt.Sprintf("[↑ %d lines as in the previous run, from %q]", len(run), first)
}

// anchors reports a line distinct enough to start a run: not blank, not a
// bare brace and not a pointer.
func anchors(l string) bool {
	t := strings.TrimSpace(l)
	return len(t) >= 4 && !strings.HasPrefix(t, "[↑ ") && !legend.Is(l)
}

func size(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}
