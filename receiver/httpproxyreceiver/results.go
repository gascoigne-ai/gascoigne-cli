package httpproxyreceiver

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// call is the tool call a result answers.
type call struct {
	name  string
	input map[string]any
}

// hintRe finds output gascoigne already optimized (exec, hooks, MCP).
var hintRe = lazyre.New(`\[(gascoigne: )?optimized \d+→\d+ tokens · full: `)

// result returns the text the model should get for one tool result; the
// same input always yields the same output, from the cache once seen.
func (r *Receiver) result(ctx context.Context, c call, text string, m meta) string {
	skip := slices.Contains(r.cfg.Skip, c.name)
	out := text
	if strings.TrimSpace(text) != "" && !skip && !hintRe.MatchString(text) {
		out = r.shorten(ctx, c, text, m)
	}
	if m.folder == nil {
		return out
	}
	folded, ok := m.folder.fold(label(c), out, !skip)
	if !ok {
		return out
	}
	return r.keep(ctx, c, out, folded, m)
}

// shorten runs one result through the pipeline, remembered by content.
func (r *Receiver) shorten(ctx context.Context, c call, text string, m meta) string {
	key := cacheKey(c, text)
	if out, ok := r.cache.get(key); ok {
		return out
	}
	out := r.optimize(ctx, c, text, m)
	r.cache.put(key, out)
	return out
}

func cacheKey(c call, text string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%v\x00", c.name, mcptool.Attrs(c.name, c.input))
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}

func (r *Receiver) optimize(ctx context.Context, c call, original string, m meta) string {
	text := original
	numbered, plain, nums := splitNumbers(text)
	if numbered {
		text = plain
	}
	p := payload.New(payload.ToolResult, text)
	for k, v := range mcptool.Attrs(c.name, c.input) {
		p.Set(k, v)
	}
	if m.model != "" {
		p.Set(payload.AttrModel, m.model)
	}
	if m.agent != "" {
		p.Set(payload.AttrAgent, m.agent)
	}
	out, err := r.next.Process(ctx, p)
	if err != nil || out == nil || out.Attrs[payload.AttrRawID] == "" {
		if err != nil {
			r.log.Warn("optimize tool result", "tool", c.name, "err", err)
		}
		return original
	}
	opt := out.Text()
	if numbered {
		opt = renumber(text, nums, opt)
	}
	return fmt.Sprintf("%s\n[gascoigne: optimized %s→%s tokens · full: gascoigne raw %s]",
		strings.TrimRight(opt, "\n"), out.Attrs[payload.AttrRawTokens], out.Attrs[payload.AttrOutTokens], out.Attrs[payload.AttrRawID])
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

// cache is a bounded LRU of transformed tool results.
type cache struct {
	mu    sync.Mutex
	max   int
	order *list.List
	items map[string]*list.Element
}

type entry struct{ key, val string }

func newCache(max int) *cache {
	return &cache{max: max, order: list.New(), items: map[string]*list.Element{}}
}

func (c *cache) get(k string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[k]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(e)
	return e.Value.(*entry).val, true
}

func (c *cache) put(k, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.items[k]; ok {
		e.Value.(*entry).val = v
		c.order.MoveToFront(e)
		return
	}
	c.items[k] = c.order.PushFront(&entry{k, v})
	for c.order.Len() > c.max {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*entry).key)
	}
}
