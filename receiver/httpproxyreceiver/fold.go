package httpproxyreceiver

import (
	"cmp"
	"context"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/internal/hint"
	"strconv"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// folder replaces runs of a tool result that an earlier result of the same
// request already shows with one pointer line: agents re-read files and
// re-run commands, and the model still holds the first copy. A result's
// pointers depend only on the results before it, so earlier bytes never
// change and the prompt cache holds. One folder serves one request.
type folder struct {
	minLines, minChars int
	results            []shown
	anchors            map[string][]spot // line key → first places it is shown
}

// shown is one earlier result: its label and lines, nil where folded.
type shown struct {
	label string
	lines []*line
}

type spot struct{ result, line int }

// line splits off a Read line number ("  40\tcode": num 40, key "code").
type line struct {
	num int // -1 when not numbered
	key string
	raw string
}

// maxAnchors caps the places tried per line, so a common line cannot make
// folding quadratic.
const maxAnchors = 16

func newFolder(minLines, minChars int) *folder {
	return &folder{minLines: minLines, minChars: minChars, anchors: map[string][]spot{}}
}

// fold returns text with runs shown earlier replaced by pointers, and
// remembers what stays for later results. touch false only remembers.
func (f *folder) fold(label, text string, touch bool) (string, bool) {
	cur := parse(text)
	kept := make([]*line, 0, len(cur))
	var out []string
	for i := 0; i < len(cur); {
		if touch {
			if at, n, ok := f.match(cur, i); ok {
				out = append(out, f.pointer(at, cur[i:i+n]))
				kept = append(kept, make([]*line, n)...)
				i += n
				continue
			}
		}
		out = append(out, cur[i].raw)
		kept = append(kept, cur[i])
		i++
	}
	f.remember(label, kept)
	folded := strings.Join(out, "\n")
	return folded, folded != text
}

// match finds the longest run from cur[i] shown earlier, earliest first on
// ties; ok when it is long and big enough to be worth a pointer.
func (f *folder) match(cur []*line, index int) (at spot, n int, ok bool) {
	for _, s := range f.anchors[cur[index].key] {
		ref := f.results[s.result].lines
		k, delta := 0, 0
		for index+k < len(cur) && s.line+k < len(ref) && same(ref[s.line+k], cur[index+k]) {
			if d := cur[index+k].num - ref[s.line+k].num; k == 0 {
				delta = d
			} else if d != delta {
				break // an edit inside the run: the rest is not the same text
			}
			k++
		}
		if k > n {
			at, n = s, k
		}
	}
	if n < f.minLines {
		return at, n, false
	}
	size := 0
	for _, l := range cur[index : index+n] {
		size += len(l.raw) + 1
	}
	return at, n, size >= f.minChars && size > len(f.pointer(at, cur[index:index+n]))
}

// same compares a shown line with a new one: numbered lines by text (their
// numbers may shift by a constant after an edit), others byte for byte.
func same(ref, cur *line) bool {
	if ref == nil || (ref.num < 0) != (cur.num < 0) {
		return false
	}
	return ref.key == cur.key
}

// pointer names the earlier result and where the run is in it: line numbers
// for numbered reads, else its first line.
func (f *folder) pointer(at spot, run []*line) string {
	r := f.results[at.result]
	first, last := r.lines[at.line], r.lines[at.line+len(run)-1]
	if first.num < 0 {
		return fmt.Sprintf("[↑ %d lines as in earlier %s from %q]", len(run), r.label, clip(strings.TrimSpace(first.raw), 40))
	}
	was := fmt.Sprintf("L%d-%d", first.num, last.num)
	if run[0].num == first.num {
		return fmt.Sprintf("[↑ %s as in earlier %s]", was, r.label)
	}
	return fmt.Sprintf("[↑ L%d-%d = %s of earlier %s]", run[0].num, run[len(run)-1].num, was, r.label)
}

func (f *folder) remember(label string, lines []*line) {
	idx := len(f.results)
	f.results = append(f.results, shown{label, lines})
	for i, l := range lines {
		if l == nil || !anchors(l.key) {
			continue
		}
		if s := f.anchors[l.key]; len(s) < maxAnchors {
			f.anchors[l.key] = append(s, spot{idx, i})
		}
	}
}

// anchors reports a line distinct enough to start a run: not blank or a
// bare brace, not a pointer or gascoigne hint.
func anchors(key string) bool {
	t := strings.TrimSpace(key)
	return len(t) >= 4 && !strings.HasPrefix(t, "[↑ ") && !strings.HasPrefix(t, "[gascoigne: ")
}

func parse(text string) []*line {
	raw := strings.Split(text, "\n")
	lines := make([]*line, len(raw))
	for i, r := range raw {
		lines[i] = &line{num: -1, key: r, raw: r}
		if m := numberedLine.FindStringSubmatch(r); m != nil {
			n, _ := strconv.Atoi(m[1])
			lines[i].num, lines[i].key = n, m[2]
		}
	}
	return lines
}

// clip cuts s to n runes with an ellipsis.
func clip(text string, limit int) string {
	if r := []rune(text); len(r) > limit {
		return string(r[:limit-1]) + "…"
	}
	return text
}

// label names a call for pointers: Read main.go, Bash "go test ./...".
func label(toolCall call) string {
	a := mcptool.Attrs(toolCall.name, toolCall.input)
	if f := a[payload.AttrFile]; f != "" {
		return toolCall.name + " " + clip(f, 60)
	}
	if cmd := a[payload.AttrCmd]; cmd != "" {
		return fmt.Sprintf("%s %q", toolCall.name, clip(cmd, 50))
	}
	return toolCall.name
}

// keep returns the folded result when the original before folding is in the
// ledger (content-addressed, so the hint stays the same in later requests)
// and folding saves more than the hint costs; else the result unfolded.
func (r *Receiver) keep(ctx context.Context, toolCall call, before, folded string, info meta) string {
	if r.ledger == nil {
		return before
	}
	key := "fold\x00" + cacheKey(toolCall, before) + cacheKey(toolCall, folded)
	if out, ok := r.cache.Get(key); ok {
		return out
	}
	p := payload.New(payload.ToolResult, before)
	for k, v := range mcptool.Attrs(toolCall.name, toolCall.input) {
		p.Set(k, v)
	}
	out, err := r.save(ctx, p, folded, info)
	if err != nil {
		r.log.Warn("fold tool result", "tool", toolCall.name, "err", err)
		return before
	}
	r.cache.Put(key, out)
	return out
}

// minSaving is the fewest tokens a fold must save: a hint costs about that.
const minSaving = 20

func (r *Receiver) save(ctx context.Context, data *payload.Payload, folded string, info meta) (string, error) {
	in := tokens.Estimate(data.Text())
	if in-tokens.Estimate(folded) < 2*minSaving {
		return data.Text(), nil
	}
	id, err := r.ledger.SaveRaw(ctx, data.Key(), data)
	if err != nil {
		return "", err
	}
	out := folded
	if !hint.Re.MatchString(folded) {
		out = strings.TrimRight(folded, "\n") + "\n" + hint.Of(id)
	}
	n := tokens.Estimate(out)
	err = r.ledger.Record(ctx, component.Event{
		TS: time.Now(), Agent: info.agent, Model: info.model, Pipeline: "proxy", Key: data.Key(), Route: "fold",
		Command:    cmp.Or(detect.Command(data.Attrs[payload.AttrCmd]), data.Attrs[payload.AttrTool]),
		Optimizers: []string{"fold"}, RawTokens: in, OutTokens: n, RawID: id, Outcome: component.Optimized,
		Steps: []component.Step{{Optimizer: "fold", Family: "core", RawTokens: in, OutTokens: n}}, Output: out,
	})
	return out, err
}
