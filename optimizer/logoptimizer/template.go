package logoptimizer

import (
	"fmt"
	"math"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// entry is one output line and the sources (container prefixes) it came
// from: none without prefixes and for markers, several for merged repeats.
type entry struct {
	text string
	srcs []int
	// solo marks an outlier: it never merges into a run of its neighbors.
	solo bool
	// n is how many input lines the entry stands for; 0 for markers. The
	// fewer, the rarer its template.
	n int
}

func texts(es []entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.text
	}
	return out
}

// folding is the state template builds its output from: per line whether it
// must be kept, and per repeat key how often and where the key occurs.
type folding struct {
	lines     []string
	src       []int
	important []bool
	outlier   []bool
	keys      []string
	notes     map[string]string
	counts    map[string]int
	srcsOf    map[string][]int
	at        map[string][]int
}

// sources is the source of line index as a list, nil when there are none.
func (f *folding) sources(index int) []int {
	if f.src == nil {
		return nil
	}
	return []int{f.src[index]}
}

// template templates lines; src[i] is line i's source, nil when there are none.
func (o *Optimizer) template(lines []string, src []int) []entry {
	f := &folding{lines: lines, src: src, important: o.importantLines(lines), outlier: make([]bool, len(lines))}
	norms, vols := o.normalizeAll(lines)
	for _, i := range o.outliers(lines, norms, vols) {
		f.important[i], f.outlier[i] = true, true
	}
	f.keys, f.notes = o.repeatKeys(lines, norms, f.important)
	o.countRepeats(f)
	return o.limit(o.dedupeRuns(o.foldLines(f)))
}

// importantLines marks the lines that are never folded: warnings, errors,
// their context and their continuation lines.
func (o *Optimizer) importantLines(lines []string) []bool {
	important := make([]bool, len(lines))
	for i, l := range lines {
		if severity(l) == compress.None {
			continue
		}
		for j := max(0, i-o.cfg.Context); j <= min(len(lines)-1, i+o.cfg.Context); j++ {
			important[j] = true
		}
		for j := i + 1; j < len(lines) && continuation.MatchString(lines[j]); j++ {
			important[j] = true
		}
	}
	return important
}

// countRepeats fills counts, srcsOf and at for the lines that may fold.
func (o *Optimizer) countRepeats(fold *folding) {
	fold.counts = map[string]int{}
	fold.srcsOf = map[string][]int{}
	fold.at = map[string][]int{}
	for i := range fold.lines {
		if fold.important[i] {
			continue
		}
		key := fold.keys[i]
		fold.counts[key]++
		if o.cfg.Bursts {
			fold.at[key] = append(fold.at[key], i)
		}
		if fold.src != nil {
			fold.srcsOf[key] = union(fold.srcsOf[key], fold.sources(i))
		}
	}
}

// foldLines keeps the first line of each repeat key and the important
// lines, and puts a gap marker where lines were dropped.
func (o *Optimizer) foldLines(fold *folding) []entry {
	var out []entry
	seen := map[string]bool{}
	dropped, repeats := 0, 0
	for i, l := range fold.lines {
		norm := fold.keys[i]
		keep := fold.important[i] || (!seen[norm] && !(o.cfg.DropDebug && isDebug(l)))
		if !keep {
			dropped++
			if seen[norm] {
				repeats++
			}
			continue
		}
		if dropped > 0 {
			out = append(out, entry{text: gap(dropped, repeats)})
			dropped, repeats = 0, 0
		}
		if !fold.important[i] {
			seen[norm] = true
		}
		out = append(out, o.keptEntry(fold, i))
	}
	if dropped > 0 {
		out = append(out, entry{text: gap(dropped, repeats)})
	}
	return out
}

// keptEntry is the output entry of the line at index, with its ×N note when it stands
// for repeats.
func (o *Optimizer) keptEntry(fold *folding, index int) entry {
	l := fold.lines[index]
	e := entry{text: l, srcs: fold.sources(index), n: 1, solo: fold.outlier[index]}
	if fold.important[index] {
		return e
	}
	norm := fold.keys[index]
	e.srcs = fold.srcsOf[norm]
	e.n = fold.counts[norm]
	if n := fold.counts[norm]; n > 1 {
		e.text = l + " (" + strings.Join(append([]string{fmt.Sprintf("×%d", n)}, o.repeatNotes(fold.notes[norm], fold.at[norm], len(fold.lines))...), "; ") + ")"
	}
	return e
}

const (
	// textMinLines is the fewest lines worth a template pass on unknown text.
	textMinLines = 20
	// textMaxRatio is the most distinct templates per line: prose is near 1,
	// repetitive listings and traces well below.
	textMaxRatio = 0.5
)

// stampRe is what varies between repeats of one text event. Plain numbers and
// hashes do not count: rows of data (dumps, checksum lists) differ by them.
var stampRe = lazyre.New(`\d{4}-\d\d-\d\d[T ]\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:?\d\d)?|\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

// foldText runs the template engine on unknown text, only when it is long and
// repetitive in timestamps and ids alone. Debug lines and the line cap do not
// apply: text is not a log.
func (o *Optimizer) foldText(lines []string) []string {
	t := &Optimizer{
		cfg:  &Config{Context: o.cfg.Context, MaxLines: math.MaxInt},
		norm: func(line string) string { return stampRe.ReplaceAllString(line, "#") },
	}
	norms := map[string]bool{}
	n := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			norms[t.normalize(l)] = true
			n++
		}
	}
	if n < textMinLines || float64(len(norms)) > textMaxRatio*float64(n) {
		return lines
	}
	return texts(t.template(lines, nil))
}
