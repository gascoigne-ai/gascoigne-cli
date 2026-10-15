package dboptimizer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	planNode   = lazyre.New(`^(\s*)(->\s+)?([A-Z][\w .-]+?)(?: on (\S+)(?: (\S+))?)?\s+\(cost=[\d.]+\.\.([\d.]+) rows=(\d+) width=\d+\)(?: \(actual time=[\d.]+\.\.([\d.]+) rows=(\d+) loops=(\d+)\))?`)
	planDetail = lazyre.New(`^\s+(Filter|Index Cond|Hash Cond|Join Filter|Recheck Cond|Sort Key|Rows Removed by Filter|Group Key|Merge Cond|Output|Sort Method|Heap Fetches):`)
	planNoise  = lazyre.New(`^\s+(Buffers:|Worker \d+:|Planning:|I/O Timings:|Memory Usage:|Batches:)`)
)

type node struct {
	line    string
	details []string
	time    float64 // actual total time, else cost
	depth   int
}

// plan keeps the nodes that matter: the slow ones (≥ PlanMinShare of the
// slowest) and their filters; cheap subtrees collapse into a count. Costs
// give way to actual times and row counts when present.
func (o *Optimizer) plan(lines []string) []string {
	var nodes []*node
	var head, tail []string
	for _, l := range lines {
		if m := planNode.FindStringSubmatch(l); m != nil {
			nodes = append(nodes, &node{line: shorten(m), time: timeOf(m), depth: len(m[1])})
			continue
		}
		switch {
		case planNoise.MatchString(l):
		case len(nodes) > 0 && planDetail.MatchString(l):
			nodes[len(nodes)-1].details = append(nodes[len(nodes)-1].details, strings.TrimSpace(l))
		case len(nodes) == 0:
			head = append(head, l)
		default:
			tail = append(tail, l)
		}
	}
	slowest := 0.0
	for _, n := range nodes {
		slowest = max(slowest, n.time)
	}
	out := head
	cheap := 0
	for i, n := range nodes {
		if i > 0 && n.time < o.cfg.PlanMinShare*slowest {
			cheap++
			continue
		}
		if cheap > 0 {
			out = append(out, strings.Repeat(" ", n.depth)+compress.Omitted(cheap, "cheap plan nodes"))
			cheap = 0
		}
		out = append(out, n.line)
		for _, d := range n.details {
			out = append(out, strings.Repeat(" ", n.depth+4)+d)
		}
	}
	if cheap > 0 {
		out = append(out, compress.Omitted(cheap, "cheap plan nodes"))
	}
	return append(out, tail...)
}

// shorten rewrites a node as "-> Seq Scan on orders o (time=12.5ms rows=1000 est=10 loops=1)".
func shorten(m []string) string {
	s := m[1] + m[2] + strings.TrimSpace(m[3])
	if m[4] != "" {
		s += " on " + m[4]
		if m[5] != "" {
			s += " " + m[5]
		}
	}
	if m[8] != "" {
		return s + fmt.Sprintf(" (time=%sms rows=%s est=%s loops=%s)", m[8], m[9], m[7], m[10])
	}
	return s + fmt.Sprintf(" (cost=%s rows=%s)", m[6], m[7])
}

func timeOf(m []string) float64 {
	v := m[6]
	if m[8] != "" {
		v = m[8]
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}
