// Package ariaoptimizer is the web family engine for accessibility snapshots
// of browser automation tools: role trees whose [ref=eN] ids the agent
// clicks and types into. Wrappers that only group (generic, none,
// presentation; a listitem around one node) go and their children move up,
// [cursor=pointer] is dropped where the role is clickable anyway, and long
// runs of like siblings keep head, tail and the ones matching the query.
// Text outside the tree (page URL, title, code) stays as it is.
package ariaoptimizer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := sample.Terms(p.Attrs[payload.AttrQuery])
	return p.Rewrite(func(b payload.Block) (string, error) {
		return o.shrink(b.Text, query), nil
	}, detect.ARIA)
}

// node is one tree line ("link "Home" [ref=e3]:" without indent and dash),
// lines continuing it, and its children.
type node struct {
	text  string
	extra []string
	kids  []*node
}

var (
	// wrapper is a node that only groups its children.
	wrapper = lazyre.New(`^(generic|none|presentation)( \[ref=e\d+\])?:$`)
	// single is a node that only wraps one child.
	single = lazyre.New(`^listitem( \[ref=e\d+\])?:$`)
	// clickable roles say "you can click me" without [cursor=pointer].
	clickable = lazyre.New(`^(link|button|checkbox|radio|tab|menuitem|menuitemcheckbox|menuitemradio|option|switch|combobox|treeitem)\b`)
)

// shrink rewrites each tree in text; other lines pass through.
func (o *Optimizer) shrink(text string, query []string) string {
	var out []string
	var roots []*node
	var stack []*node
	var depths []int
	flush := func() {
		out = append(out, render(o.simplify(roots, query), 0)...)
		roots, stack, depths = nil, nil, nil
	}
	for _, l := range compress.Lines(text) {
		trimmed := strings.TrimLeft(l, " ")
		indent := len(l) - len(trimmed)
		if !strings.HasPrefix(trimmed, "- ") {
			if len(stack) > 0 && indent > depths[len(depths)-1] && trimmed != "" {
				top := stack[len(stack)-1]
				top.extra = append(top.extra, trimmed)
				continue
			}
			flush()
			out = append(out, l)
			continue
		}
		for len(depths) > 0 && depths[len(depths)-1] >= indent {
			stack, depths = stack[:len(stack)-1], depths[:len(depths)-1]
		}
		n := &node{text: trimmed[2:]}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			parent := stack[len(stack)-1]
			parent.kids = append(parent.kids, n)
		}
		stack, depths = append(stack, n), append(depths, indent)
	}
	flush()
	return compress.Join(out)
}

// simplify unwraps wrappers, drops redundant attributes and folds long runs.
func (o *Optimizer) simplify(nodes []*node, query []string) []*node {
	var out []*node
	for _, n := range nodes {
		n.kids = o.simplify(n.kids, query)
		if len(n.kids) > 0 && len(n.extra) == 0 && (wrapper.MatchString(n.text) || single.MatchString(n.text) && len(n.kids) == 1) {
			out = append(out, n.kids...)
			continue
		}
		if clickable.MatchString(n.text) {
			n.text = strings.Replace(n.text, " [cursor=pointer]", "", 1)
		}
		out = append(out, n)
	}
	return o.fold(out, query)
}

// fold keeps head, tail and the best query matches of each run of over
// MaxItems siblings with the same role; a marker stands for each gap.
func (o *Optimizer) fold(nodes []*node, query []string) []*node {
	var out []*node
	for i := 0; i < len(nodes); {
		j := i
		for j < len(nodes) && role(nodes[j]) == role(nodes[i]) {
			j++
		}
		run := nodes[i:j]
		i = j
		if len(run) <= o.cfg.MaxItems {
			out = append(out, run...)
			continue
		}
		match := matches(run, query)
		gap := 0
		for k, n := range run {
			if k < o.cfg.Head || k >= len(run)-o.cfg.Tail || match[k] {
				if gap > 0 {
					out = append(out, marker(gap, role(run[0])))
					gap = 0
				}
				out = append(out, n)
				continue
			}
			gap++
		}
	}
	return out
}

func marker(n int, role string) *node {
	return &node{text: compress.Omitted(n, role+"s")}
}

// role is a node's first word: "link" of link "Home" [ref=e3].
func role(n *node) string {
	r, _, _ := strings.Cut(n.text, " ")
	return strings.TrimSuffix(r, ":")
}

// matches marks the nodes of run whose subtree scores at least half the
// best BM25 match of the query: words every sibling shares weigh little.
func matches(run []*node, query []string) []bool {
	match := make([]bool, len(run))
	if len(query) == 0 {
		return match
	}
	docs := make([][]string, len(run))
	for i, n := range run {
		docs[i] = sample.Terms(strings.Join(render([]*node{n}, 0), "\n"))
	}
	scores := sample.BM25(docs, query)
	best := slices.Max(scores)
	for i, sc := range scores {
		match[i] = sc > 0 && sc >= best/2
	}
	return match
}

func render(nodes []*node, depth int) []string {
	var out []string
	pad := strings.Repeat("  ", depth)
	for _, n := range nodes {
		out = append(out, fmt.Sprintf("%s- %s", pad, n.text))
		for _, e := range n.extra {
			out = append(out, pad+"  "+e)
		}
		out = append(out, render(n.kids, depth+1)...)
	}
	return out
}
