package logoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

// Container log prefixes put the source in front of every line (kubectl
// logs --prefix, compose logs), which hides levels, stack-frame indentation
// and repeats from the engine. They are cut off, the lines templated, and
// each kept line labeled with its sources again.
var prefixes = []*lazyre.Regexp{
	lazyre.New(`^\[((?:pod|deployment|statefulset|daemonset|job|replicaset)/[^\]\s]+)\] ?`),
	lazyre.New(`^([A-Za-z0-9][\w.-]*)\s+\| ?`),
}

// sourced is a log split from its prefixes: line i came from names[src[i]].
type sourced struct {
	lines []string
	src   []int
	names []string
}

// maxSources is the most labels a log gets; more sources stay as they are.
const maxSources = 26

// split cuts the source prefix off every line when at least 80% of the
// non-blank lines carry one; lines without (wrapped output) belong to the
// source above.
func split(lines []string) (sourced, bool) {
	for _, re := range prefixes {
		if s, ok := splitBy(lines, re); ok {
			return s, true
		}
	}
	return sourced{}, false
}

func splitBy(lines []string, re *lazyre.Regexp) (sourced, bool) {
	s := sourced{lines: make([]string, len(lines)), src: make([]int, len(lines))}
	index := map[string]int{}
	cur, matched, nonblank := 0, 0, 0
	for i, l := range lines {
		s.lines[i] = l
		if strings.TrimSpace(l) != "" {
			nonblank++
		}
		if m := re.FindStringSubmatchIndex(l); m != nil {
			name := l[m[2]:m[3]]
			n, ok := index[name]
			if !ok {
				n = len(s.names)
				index[name] = n
				s.names = append(s.names, name)
			}
			cur, s.lines[i] = n, l[m[1]:]
			matched++
		}
		s.src[i] = cur
	}
	ok := matched >= 3 && matched*5 >= nonblank*4 && len(s.names) <= maxSources
	return s, ok
}

// labels name the sources in output: short names as they are, long ones
// (pod names with hashes) as letters defined in a legend line.
func labels(names []string) (label []string, legend string) {
	var defs []string
	letter := 'A'
	for _, n := range names {
		if len(n) <= 12 {
			label = append(label, n)
			continue
		}
		label = append(label, string(letter))
		defs = append(defs, fmt.Sprintf("%c=%s", letter, n))
		letter++
	}
	if len(defs) > 0 {
		legend = "[sources: " + strings.Join(defs, " ") + "]"
	}
	return label, legend
}

// render puts labels back: one source is named once on top, several label
// each line ("A| text", "A+B| text" for repeats from both).
func render(es []entry, names []string) []string {
	if len(names) == 1 {
		return append([]string{"[source: " + names[0] + "]"}, texts(es)...)
	}
	label, legend := labels(names)
	var out []string
	if legend != "" {
		out = append(out, legend)
	}
	for _, e := range es {
		if len(e.srcs) == 0 {
			out = append(out, e.text)
			continue
		}
		var ls []string
		for _, s := range e.srcs {
			ls = append(ls, label[s])
		}
		out = append(out, strings.Join(ls, "+")+"| "+e.text)
	}
	return out
}
