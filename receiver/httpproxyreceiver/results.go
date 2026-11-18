package httpproxyreceiver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/internal/hint"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// call is the tool call a result answers.
type call struct {
	name  string
	input map[string]any
}

// result returns the text the model should get for one tool result; the
// same input always yields the same output, from the cache once seen.
func (r *Receiver) result(ctx context.Context, toolCall call, text string, info meta) string {
	skip := slices.Contains(r.cfg.Skip, toolCall.name)
	out := text
	if strings.TrimSpace(text) != "" && !skip && !hint.Re.MatchString(text) {
		out = r.shorten(ctx, toolCall, text, info)
	}
	if info.folder == nil {
		return out
	}
	folded, ok := info.folder.fold(label(toolCall), out, !skip)
	if !ok {
		return out
	}
	return r.keep(ctx, toolCall, out, folded, info)
}

// shorten runs one result through the pipeline, remembered by content.
func (r *Receiver) shorten(ctx context.Context, toolCall call, text string, info meta) string {
	key := cacheKey(toolCall, text)
	if out, ok := r.cache.Get(key); ok {
		return out
	}
	out := r.optimize(ctx, toolCall, text, info)
	r.cache.Put(key, out)
	return out
}

func cacheKey(toolCall call, text string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%v\x00", toolCall.name, mcptool.Attrs(toolCall.name, toolCall.input))
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

func (r *Receiver) optimize(ctx context.Context, toolCall call, original string, info meta) string {
	text := original
	numbered, plain, nums := splitNumbers(text)
	if numbered {
		text = plain
	}
	p := payload.New(payload.ToolResult, text)
	for k, v := range mcptool.Attrs(toolCall.name, toolCall.input) {
		p.Set(k, v)
	}
	if info.model != "" {
		p.Set(payload.AttrModel, info.model)
	}
	if info.agent != "" {
		p.Set(payload.AttrAgent, info.agent)
	}
	out, err := r.next.Process(ctx, p)
	if err != nil || out == nil || out.Attrs[payload.AttrRawID] == "" {
		if err != nil {
			r.log.Warn("optimize tool result", "tool", toolCall.name, "err", err)
		}
		return original
	}
	opt := out.Text()
	if numbered {
		opt = renumber(text, nums, opt)
	}
	return strings.TrimRight(opt, "\n") + "\n" + hint.Of(out.Attrs[payload.AttrRawID])
}

// Claude Code renders Read results as "N\ttext" lines.
var numberedLine = lazyre.New(`^ *(\d+)\t(.*)$`)

// splitNumbers strips "N\t" prefixes when nearly every line has one; nums
// keeps them per line ("" for lines without, such as a trailing note).
func splitNumbers(text string) (bool, string, []string) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	plain := make([]string, len(lines))
	nums := make([]string, len(lines))
	hits := 0
	for i, l := range lines {
		if m := numberedLine.FindStringSubmatch(l); m != nil {
			nums[i], plain[i] = m[1], m[2]
			hits++
			continue
		}
		plain[i] = l
	}
	if hits < 5 || hits*10 < len(lines)*9 {
		return false, "", nil
	}
	return true, strings.Join(plain, "\n") + "\n", nums
}

// renumber gives the optimized lines their original numbers back: kept
// lines are byte-exact and in order, so walking both finds each one's
// number; lines the optimizer wrote (markers like "// … L12-30") get none.
func renumber(plain string, nums []string, opt string) string {
	orig := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	var b strings.Builder
	i := 0
	for _, l := range strings.Split(strings.TrimRight(opt, "\n"), "\n") {
		j := i
		for j < len(orig) && orig[j] != l {
			j++
		}
		if j < len(orig) && nums[j] != "" {
			fmt.Fprintf(&b, "%s\t%s\n", nums[j], l)
			i = j + 1
			continue
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}
