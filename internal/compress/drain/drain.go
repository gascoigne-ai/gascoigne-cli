// Package drain mines log templates online, after Drain (He et al., 2017):
// lines go down a fixed-depth tree keyed on token count and leading tokens,
// then join the most similar cluster in the leaf or start a new one. Unlike
// the exact key of compress.Normalize, lines that differ in a few words
// (a host name, a user) share a cluster. Output depends only on line order.
package drain

import (
	"strconv"
	"strings"
)

// wildcard marks a token position that varies inside a cluster.
const wildcard = "<*>"

// Config tunes the miner.
type Config struct {
	// Depth is the tree depth: the length level, then Depth-2 leading tokens.
	Depth int
	// Similarity is the share of equal token positions a line needs to join
	// a cluster, in (0,1].
	Similarity float64
	// MaxChildren caps the branches of one tree node; later tokens share a
	// wildcard branch so variable leading tokens cannot blow the tree up.
	MaxChildren int
	// MaxVars is the most word positions one cluster may vary in; lines that
	// would make it vary in more start a new cluster, so free text (unit
	// names, messages) is not folded into one template. 0 means no limit.
	MaxVars int
}

type cluster struct {
	id     int
	tokens []string
}

type node struct {
	children map[string]*node
	clusters []*cluster // leaves only
}

// Miner assigns each added line a cluster id.
type Miner struct {
	cfg      Config
	root     *node
	clusters []*cluster
}

func New(cfg Config) *Miner {
	return &Miner{cfg: cfg, root: &node{}}
}

// Add puts the line in a cluster and returns its id. The line must be masked
// already (compress.Normalize or Split): callers have it, and masking again
// would scan every line twice. Lines with different
// tags never share a cluster; tag is how a caller keeps, say, debug lines
// apart from the rest. Ids count from 0 in order of first appearance.
func (m *Miner) Add(tag, line string) int {
	toks := tokens(line)
	leaf := m.leaf(tag, toks)
	var best *cluster
	bestSim := -1.0
	for _, c := range leaf.clusters {
		if m.cfg.MaxVars > 0 && wordVars(c.tokens, toks) > m.cfg.MaxVars {
			continue
		}
		if s := similarity(c.tokens, toks); s > bestSim {
			best, bestSim = c, s
		}
	}
	if best != nil && bestSim >= m.cfg.Similarity {
		merge(best.tokens, toks)
		return best.id
	}
	c := &cluster{id: len(m.clusters), tokens: toks}
	m.clusters = append(m.clusters, c)
	leaf.clusters = append(leaf.clusters, c)
	return c.id
}

// Words returns the words of the masked line at the places where cluster id varies:
// what tells the lines of one template apart, apart from numbers and ids. Call
// it after every line is added, when the template has settled.
func (m *Miner) Words(id int, line string) []string {
	toks := tokens(line)
	var out []string
	for i, t := range m.clusters[id].tokens {
		if t == wildcard && !strings.Contains(toks[i], "#") {
			out = append(out, toks[i])
		}
	}
	return out
}

// tokens splits a masked line; masked numbers and ids hold a "#" and never
// count against similarity.
func tokens(line string) []string {
	return strings.Fields(line)
}

// leaf finds, creating the path as needed, the leaf for tag and tokens.
func (m *Miner) leaf(tag string, toks []string) *node {
	n := m.root.child(tag+"\x00"+strconv.Itoa(len(toks)), m.cfg.MaxChildren)
	for i := 0; i < min(m.cfg.Depth-2, len(toks)); i++ {
		key := toks[i]
		if strings.Contains(key, "#") {
			key = wildcard
		}
		n = n.child(key, m.cfg.MaxChildren)
	}
	return n
}

// child returns the branch for key; once max branches exist, new keys share
// the wildcard branch.
func (n *node) child(key string, max int) *node {
	if n.children == nil {
		n.children = map[string]*node{}
	}
	if c, ok := n.children[key]; ok {
		return c
	}
	if len(n.children) >= max && key != wildcard {
		return n.child(wildcard, max+1)
	}
	c := &node{}
	n.children[key] = c
	return c
}

// similarity is the share of positions where the cluster and the line hold
// the same token; wildcard positions count as equal.
func similarity(tpl, toks []string) float64 {
	if len(tpl) == 0 {
		return 1
	}
	same := 0
	for i, t := range tpl {
		if t == toks[i] || t == wildcard || strings.Contains(t, "#") && strings.Contains(toks[i], "#") {
			same++
		}
	}
	return float64(same) / float64(len(tpl))
}

// wordVars counts the positions where the cluster varies in words once the
// line joins it: those already wildcards plus those where the line differs
// by more than a number or id.
func wordVars(tpl, toks []string) int {
	n := 0
	for i, t := range tpl {
		if t == wildcard || t != toks[i] && !(strings.Contains(t, "#") && strings.Contains(toks[i], "#")) {
			n++
		}
	}
	return n
}

// merge turns the positions where the line differs from the cluster into wildcards.
func merge(tpl, toks []string) {
	for i, t := range tpl {
		if t != toks[i] {
			tpl[i] = wildcard
		}
	}
}
