// Package tree shortens dependency trees drawn with box glyphs (npm ls,
// pnpm list, pipdeptree, uv tree, poetry show --tree) and edge lists (go mod
// graph). Leaf children of a package share one line, repeats of subtrees shown
// elsewhere (`deduped`, `(*)`) are dropped unless they match the query (then
// `=name`), a version is written at its first use only, identical subtrees
// are named once, and branches deeper than MaxDepth are counted. Branches
// that lead to a query term are never cut.
package tree

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Options tune a run.
type Options struct {
	MaxDepth int      // deepest package that gets its own line (root = 0)
	Query    []string // lower-case terms: matching packages and their path stay
}

// Legend explains the one notation that needs it; it heads output that uses it.
const Legend = "[repeats (deduped) omitted. name alone: the version listed first]"

const (
	minGlyphLines = 3   // glyph lines that make text a tree
	wrapAt        = 110 // longer lists continue on the next line
)

var (
	entry     = lazyre.New("^([ │├└─┬┼┴|+\\\\`-]*)(.*)$")
	glyphs    = lazyre.New("[│├└┬|+\\\\`]")
	repeatTag = lazyre.New(` (deduped|\(\*\)|\[deduped\])$`)
	// pipdeptree: `name [required: >=1, installed: 1.0, extra: x]`.
	pipRequired = lazyre.New(`^(\S+) \[required: .*?, installed: ([^,\]]+)(?:, extra: ([^\]]+))?\]$`)
	// A version inside a label: `name@1.0 peer`, `name v1.0 (group: dev)`, `name 1.0`.
	atVersion    = lazyre.New(`^(@?[^@\s]+)@(\S+)(.*)$`)
	spaceVersion = lazyre.New(`^(\S+) (v?\d\S*)(.*)$`)
)

type node struct {
	label   string
	repeat  bool
	repeats []*node // repeats that match the query
	kids    []*node
	keep    bool // this node or one below matches the query
	size    int  // descendants
}

// Compress rewrites a glyph tree; lines come back unchanged when they are not
// one.
func Compress(lines []string, o Options) []string {
	roots, glyphLines := parse(lines)
	if glyphLines < minGlyphLines {
		return lines
	}
	c := &run{o: o, opened: map[string]bool{}, defined: map[string]bool{}, versions: map[string]map[string]bool{}}
	for _, r := range roots {
		c.prepare(r)
	}
	for _, r := range roots {
		c.render(r, 0)
	}
	if c.legend {
		return append([]string{Legend}, c.out...)
	}
	return c.out
}

// parse builds the forest: a line's parent is the nearest earlier line with a
// smaller label column. Lines without glyphs are roots.
func parse(lines []string) (roots []*node, glyphLines int) {
	type open struct {
		col int
		n   *node
	}
	var stack []open
	for _, l := range lines {
		m := entry.FindStringSubmatch(strings.TrimRight(l, " \t"))
		prefix, label := m[1], m[2]
		switch {
		case !glyphs.MatchString(prefix):
			root := &node{label: strings.TrimSpace(l)}
			roots, stack = append(roots, root), append(stack[:0], open{0, root})
			continue
		case label == "":
			continue // a lone connector between lines
		}
		glyphLines++
		n := &node{label: label}
		if loc := repeatTag.FindStringIndex(label); loc != nil {
			n.label, n.repeat = label[:loc[0]], true
		}
		n.label = short(n.label)
		col := len([]rune(prefix))
		for len(stack) > 0 && stack[len(stack)-1].col >= col {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, n)
		} else {
			p := stack[len(stack)-1].n
			p.kids = append(p.kids, n)
		}
		stack = append(stack, open{col, n})
	}
	return roots, glyphLines
}

// short drops what the tool repeats per line: pipdeptree's `[required: …,
// installed: V]` becomes `V`, the constraint is in the original.
func short(label string) string {
	m := pipRequired.FindStringSubmatch(label)
	if m == nil {
		return label
	}
	if m[3] != "" {
		return m[1] + " " + m[2] + " (extra: " + m[3] + ")"
	}
	return m[1] + " " + m[2]
}

// run is the state of one Compress call.
type run struct {
	o        Options
	out      []string
	opened   map[string]bool            // packages whose dependencies are written
	defined  map[string]bool            // labels written, whose version later uses may drop
	versions map[string]map[string]bool // name → versions in the tree
	legend   bool
}

// prepare counts descendants and marks the nodes on a path to a query term.
func (c *run) prepare(n *node) {
	n.keep = c.matches(n.label)
	// Repeats only say that a package is listed elsewhere: dropped unless
	// they match the query.
	kids := n.kids[:0]
	for _, k := range n.kids {
		switch {
		case !k.repeat:
			kids = append(kids, k)
		case c.matches(k.label):
			n.repeats = append(n.repeats, k)
			n.keep = true
		default:
			c.legend = true
		}
	}
	n.kids = kids
	for _, k := range n.kids {
		c.prepare(k)
		n.size += 1 + k.size
		n.keep = n.keep || k.keep
	}
	if name, version, _, ok := split(n.label); ok {
		if c.versions[name] == nil {
			c.versions[name] = map[string]bool{}
		}
		c.versions[name][version] = true
	}
}

func (c *run) matches(label string) bool {
	low := strings.ToLower(label)
	return slices.ContainsFunc(c.o.Query, func(t string) bool { return strings.Contains(low, t) })
}

// split cuts a label into package name, version and what follows the version
// (`peer`, `(group: dev)`); ok is false for a label without a version.
func split(label string) (name, version, rest string, ok bool) {
	m := atVersion.FindStringSubmatch(label)
	if m == nil {
		m = spaceVersion.FindStringSubmatch(label)
	}
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

// text is label as written now: in full the first time, without its version
// after that, when the tree has no other version of the package.
func (c *run) text(label string) string {
	if !c.defined[label] {
		c.defined[label] = true
		return label
	}
	name, _, rest, ok := split(label)
	if !ok || len(c.versions[name]) > 1 {
		return label
	}
	c.legend = true
	return name + rest
}

// render writes n and what is below it, indented by depth.
func (c *run) render(n *node, depth int) {
	head := c.text(n.label)
	var leaves []string
	var branches []*node
	for _, k := range n.kids {
		if c.plain(k) {
			leaves = append(leaves, c.text(k.label))
		} else {
			branches = append(branches, k)
		}
	}
	for _, r := range n.repeats {
		leaves = append(leaves, "="+c.text(r.label))
	}
	hidden := c.cut(&branches, depth)
	if hidden > 0 {
		head += fmt.Sprintf(" (+%d deeper)", hidden)
	} else {
		c.opened[n.label] = true
	}
	c.write(depth, head, leaves)
	for _, b := range branches {
		c.branch(b, depth+1)
	}
}

// cut removes the branches past the depth cap that lead to no query term and
// returns how many packages that hides.
func (c *run) cut(branches *[]*node, depth int) (hidden int) {
	if c.o.MaxDepth == 0 || depth+1 <= c.o.MaxDepth {
		return 0
	}
	*branches = slices.DeleteFunc(*branches, func(b *node) bool {
		if b.keep {
			return false
		}
		hidden += 1 + b.size
		return true
	})
	return hidden
}

// plain reports a package that needs no line of its own: it has nothing below
// it, or its dependencies are written already (the same package under
// another parent repeats the same subtree).
func (c *run) plain(n *node) bool {
	return len(n.kids) == 0 && len(n.repeats) == 0 || c.opened[n.label] && !n.keep
}

// branch renders a package with children. A sibling earlier in the walk may
// have written the package since the parent chose its branches.
func (c *run) branch(n *node, depth int) {
	if c.plain(n) {
		c.write(depth, c.text(n.label), nil)
		return
	}
	c.render(n, depth)
}

// write adds `head: item, item`; a long list continues on lines ending in a
// comma, indented one level deeper than the package.
func (c *run) write(depth int, head string, items []string) {
	indent := strings.Repeat("  ", depth)
	if len(items) == 0 {
		c.out = append(c.out, indent+head)
		return
	}
	line := indent + head + ":"
	for i, it := range items {
		sep := " "
		if i > 0 {
			sep = ", "
		}
		if len(line)+len(sep)+len(it) > wrapAt && len(line) > len(indent)+4 {
			c.out = append(c.out, line+",")
			line, sep = indent+"    ", ""
		}
		line += sep + it
	}
	c.out = append(c.out, line)
}

var edge = lazyre.New(`^(\S+) (\S+@\S+)$`)

// Edges groups `go mod graph` lines (`module dependency`) by module:
// `module: dep, dep`. Lines come back unchanged when they are not edges.
func Edges(lines []string) []string {
	var order []string
	deps := map[string][]string{}
	for _, l := range lines {
		m := edge.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			return lines
		}
		if _, ok := deps[m[1]]; !ok {
			order = append(order, m[1])
		}
		deps[m[1]] = append(deps[m[1]], m[2])
	}
	out := make([]string, len(order))
	for i, p := range order {
		out[i] = p + ": " + strings.Join(deps[p], ", ")
	}
	return out
}
