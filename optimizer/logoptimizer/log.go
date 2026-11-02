// Package logoptimizer is the log family engine: warnings and errors stay
// with their context and stack traces, debug noise goes, other lines collapse
// to one example per template with a count. Structured cloud log events
// become one line each first; access logs become a traffic summary. Plain
// text only loses repeated lines.
package logoptimizer

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/drain"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
	// norm maps a line to its template; nil means compress.Normalize.
	norm func(string) string
}

func (o *Optimizer) normalize(l string) string {
	if o.norm == nil {
		return compress.Normalize(protectCodes(l))
	}
	return o.norm(l)
}

// normalizeAll returns every line's template and, when the default
// normalizer ran, its volatile tokens, found in the same scan.
func (o *Optimizer) normalizeAll(lines []string) (norms []string, vols [][]string) {
	norms = make([]string, len(lines))
	if o.norm != nil {
		for i, l := range lines {
			norms[i] = o.norm(l)
		}
		return norms, nil
	}
	vols = make([][]string, len(lines))
	for i, l := range lines {
		norms[i], vols[i] = compress.Split(protectCodes(l))
	}
	return norms, vols
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if b.Type != detect.Text && detect.Family(b.Type) != detect.FamilyLog {
			continue
		}
		lines := compress.CleanLines(b.Text)
		switch {
		case b.Type == detect.Text:
			lines = compress.SqueezeBlank(o.foldText(dedupeExact(lines)))
		case b.Type == detect.Access:
			lines = o.access(lines)
		default:
			// Cloud log services and JSON loggers print structured events;
			// gcloud prints YAML.
			text := strings.TrimSpace(b.Text)
			if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") || b.Type == detect.GCPLogs {
				if ev, ok := entries(text); ok {
					lines = ev
				}
			}
			if b.Type == detect.Journal || b.Type == detect.Syslog {
				lines = unitPairs(lines)
			}
			if s, ok := split(lines); ok {
				lines = render(o.template(s.lines, s.src), s.names)
				break
			}
			lines = o.log(lines)
		}
		p.Blocks[i].Text = compress.Join(lines)
	}
	return nil
}

var (
	levelRe = lazyre.New(`\b(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERROR|ERR|FATAL|CRIT|CRITICAL|PANIC)\b|\blevel=(\w+)`)
	// Continuation lines belong to the entry above: stack frames, wrapped text.
	continuation = lazyre.New(`^(\s+|at |Caused by|Traceback|\.\.\. \d+ more)`)
)

func severity(l string) compress.Severity {
	m := levelRe.FindStringSubmatch(l)
	if m == nil {
		return compress.SeverityOf(l)
	}
	switch strings.ToUpper(m[1] + m[2]) {
	case "WARN", "WARNING":
		return compress.Warn
	case "ERROR", "ERR", "FATAL", "CRIT", "CRITICAL", "PANIC":
		return compress.Error
	}
	return compress.None
}

func isDebug(l string) bool {
	m := levelRe.FindStringSubmatch(l)
	if m == nil {
		return false
	}
	lvl := strings.ToUpper(m[1] + m[2])
	return lvl == "TRACE" || lvl == "DEBUG"
}

func (o *Optimizer) log(lines []string) []string {
	return texts(o.template(lines, nil))
}

// entry is one output line and the sources (container prefixes) it came
// from: none without prefixes and for markers, several for merged repeats.
type entry struct {
	text string
	srcs []int
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

// template templates lines; src[i] is line i's source, nil when there are none.
func (o *Optimizer) template(lines []string, src []int) []entry {
	sources := func(i int) []int {
		if src == nil {
			return nil
		}
		return []int{src[i]}
	}
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

	norms, vols := o.normalizeAll(lines)
	for _, i := range outliers(lines, norms, vols, important) {
		important[i] = true
	}

	keys, notes := o.repeatKeys(lines, norms, important)
	counts := map[string]int{}
	srcsOf := map[string][]int{}
	for i := range lines {
		if !important[i] {
			counts[keys[i]]++
			if src != nil {
				srcsOf[keys[i]] = union(srcsOf[keys[i]], sources(i))
			}
		}
	}
	var out []entry
	seen := map[string]bool{}
	dropped, repeats := 0, 0
	for i, l := range lines {
		norm := keys[i]
		keep := important[i] || (!seen[norm] && !(o.cfg.DropDebug && isDebug(l)))
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
		e := entry{text: l, srcs: sources(i), n: 1}
		if !important[i] {
			seen[norm] = true
			e.srcs = srcsOf[norm]
			e.n = counts[norm]
			if n := counts[norm]; n > 1 {
				e.text = fmt.Sprintf("%s (×%d)", l, n)
				if note := notes[norm]; note != "" {
					e.text = fmt.Sprintf("%s (×%d; %s)", l, n, note)
				}
			}
		}
		out = append(out, e)
	}
	if dropped > 0 {
		out = append(out, entry{text: gap(dropped, repeats)})
	}
	return o.limit(o.dedupeRuns(out))
}

// maxNamed is how many overflow variants a folded line names.
const maxNamed = 6

// repeatKeys returns the key under which each line counts as a repeat: its
// normalized form, or with the drain miner its template cluster plus its
// words at the places the cluster varies. Only the first DrainVariants
// variants of a cluster keep a key of their own; later ones share one, and
// notes names them, so folding never hides which user or device a line was.
// Warnings, errors and outliers stay on the exact key so they never merge.
func (o *Optimizer) repeatKeys(lines, norms []string, important []bool) (keys []string, notes map[string]string) {
	if o.cfg.Miner != minerDrain || o.norm != nil {
		return norms, nil
	}
	m := drain.New(drain.Config{Depth: o.cfg.DrainDepth, Similarity: o.cfg.DrainSimilarity, MaxChildren: o.cfg.DrainMaxChildren, MaxVars: o.cfg.DrainMaxVars})
	ids := make([]int, len(lines))
	for i, l := range lines {
		if important[i] {
			continue
		}
		tag := ""
		if isDebug(l) {
			tag = "debug"
		}
		ids[i] = m.Add(tag, norms[i])
	}
	keys = slices.Clone(norms)
	owners := map[int][]string{} // cluster -> variants with a key of their own
	named := map[string][]string{}
	for i := range lines {
		if important[i] {
			continue
		}
		variant := strings.Join(m.Words(ids[i], norms[i]), " ")
		own := owners[ids[i]]
		if !slices.Contains(own, variant) && len(own) < o.cfg.DrainVariants {
			own = append(own, variant)
			owners[ids[i]] = own
		}
		key := fmt.Sprintf("\x00drain%d %s", ids[i], variant)
		if !slices.Contains(own, variant) {
			key = fmt.Sprintf("\x00drain%d more", ids[i])
			if !slices.Contains(named[key], variant) {
				named[key] = append(named[key], variant)
			}
		}
		keys[i] = key
	}
	notes = map[string]string{}
	for k, vs := range named {
		if len(vs) < 2 {
			continue // the line shown is the only variant
		}
		others := vs[1:]
		shown := others[:min(len(others), maxNamed)]
		note := "also " + strings.Join(shown, ", ")
		if extra := len(others) - len(shown); extra > 0 {
			note += fmt.Sprintf(", +%d", extra)
		}
		notes[k] = note
	}
	return keys, notes
}

// gap marks dropped lines. When each repeats a line shown above (and counted
// there with ×N), saying so tells the reader nothing new is hidden; a mixed
// run stays "omitted", the counts above already tell its repeats.
func gap(dropped, repeats int) string {
	switch {
	case repeats < dropped:
		return compress.Omitted(dropped, "")
	case dropped == 1:
		return "… 1 repeat of a line above"
	}
	return fmt.Sprintf("… %d repeats of lines above", dropped)
}

// dedupeRuns collapses consecutive entries equal after Normalize into the
// first one suffixed with "(×N)", like compress.DedupeRuns.
func (o *Optimizer) dedupeRuns(es []entry) []entry {
	norms := make([]string, len(es))
	for i, e := range es {
		norms[i] = o.normalize(e.text)
	}
	var out []entry
	for i := 0; i < len(es); {
		j := i + 1
		e := es[i]
		for j < len(es) && norms[j] == norms[i] {
			e.srcs = union(e.srcs, es[j].srcs)
			e.n += es[j].n
			j++
		}
		if n := j - i; n > 1 && strings.TrimSpace(e.text) != "" {
			e.text = fmt.Sprintf("%s (×%d)", e.text, n)
		}
		out = append(out, e)
		i = j
	}
	return out
}

// union adds b's sources to a, keeping them sorted and distinct.
func union(a, b []int) []int {
	for _, s := range b {
		if i, found := slices.BinarySearch(a, s); !found {
			a = slices.Insert(slices.Clone(a), i, s)
		}
	}
	return a
}

// rarityShare bounds what Rarity adds: MaxLines/rarityShare entries. Filling
// the whole cap nearly doubled system-log output (journalctl, dmesg) for 1-3
// more artifacts read back, since there almost every line is rare.
const rarityShare = 8

// limit keeps warnings/errors plus head and tail when still too long. With
// Rarity it adds up to MaxLines/rarityShare entries more, the ones that stand
// for the fewest lines (the highest surprisal of their template).
func (o *Optimizer) limit(es []entry) []entry {
	if len(es) <= o.cfg.MaxLines {
		return es
	}
	head, tail := o.cfg.MaxLines/4, o.cfg.MaxLines/4
	keep := make([]bool, len(es))
	kept := 0
	for i, e := range es {
		if i < head || i >= len(es)-tail || severity(e.text) != compress.None || continuation.MatchString(e.text) {
			keep[i] = true
			kept++
		}
	}
	if o.cfg.Rarity {
		for _, i := range o.rarest(es, keep, min(o.cfg.MaxLines-kept, o.cfg.MaxLines/rarityShare)) {
			keep[i] = true
		}
	}
	var out []entry
	dropped := 0
	for i, e := range es {
		if !keep[i] {
			dropped++
			continue
		}
		if dropped > 0 {
			out = append(out, entry{text: compress.Omitted(dropped, "")})
			dropped = 0
		}
		out = append(out, e)
	}
	return out
}

// rarest returns the indexes of up to room entries not kept yet, the ones
// standing for the fewest lines first, earlier first among equals. Markers
// stand for nothing and are never picked.
func (o *Optimizer) rarest(es []entry, keep []bool, room int) []int {
	var idx []int
	for i, e := range es {
		if !keep[i] && e.n > 0 {
			idx = append(idx, i)
		}
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(es[a].n, es[b].n) })
	return idx[:max(0, min(room, len(idx)))]
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
		norm: func(l string) string { return stampRe.ReplaceAllString(l, "#") },
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

// dedupeExact collapses runs of identical lines; text keeps every distinct line.
func dedupeExact(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && lines[j] == lines[i] {
			j++
		}
		if n := j - i; n > 2 && lines[i] != "" {
			out = append(out, fmt.Sprintf("%s (×%d)", lines[i], n))
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	return out
}
