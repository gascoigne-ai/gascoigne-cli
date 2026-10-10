// Package logoptimizer is the log family engine: warnings and errors stay
// with their context and stack traces, debug noise goes, other lines collapse
// to one example per template with a count. Structured cloud log events
// become one line each first; access logs become a traffic summary. Plain
// text only loses repeated lines.
package logoptimizer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if b.Type != detect.Text && detect.Family(b.Type) != detect.FamilyLog {
			continue
		}
		lines := compress.Lines(compress.StripANSI(b.Text))
		switch {
		case b.Type == detect.Text:
			lines = compress.SqueezeBlank(dedupeExact(lines))
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
	return m != nil && strings.Contains("TRACE DEBUG", strings.ToUpper(m[1]+m[2]))
}

func (o *Optimizer) log(lines []string) []string {
	return texts(o.template(lines, nil))
}

// entry is one output line and the sources (container prefixes) it came
// from: none without prefixes and for markers, several for merged repeats.
type entry struct {
	text string
	srcs []int
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

	counts := map[string]int{}
	srcsOf := map[string][]int{}
	for i, l := range lines {
		if !important[i] {
			norm := compress.Normalize(l)
			counts[norm]++
			srcsOf[norm] = union(srcsOf[norm], sources(i))
		}
	}
	var out []entry
	seen := map[string]bool{}
	dropped := 0
	for i, l := range lines {
		norm := compress.Normalize(l)
		keep := important[i] || (!seen[norm] && !(o.cfg.DropDebug && isDebug(l)))
		if !keep {
			dropped++
			continue
		}
		if dropped > 0 {
			out = append(out, entry{text: compress.Omitted(dropped, "")})
			dropped = 0
		}
		e := entry{l, sources(i)}
		if !important[i] {
			seen[norm] = true
			e.srcs = srcsOf[norm]
			if n := counts[norm]; n > 1 {
				e.text = fmt.Sprintf("%s (×%d)", l, n)
			}
		}
		out = append(out, e)
	}
	if dropped > 0 {
		out = append(out, entry{text: compress.Omitted(dropped, "")})
	}
	return o.limit(dedupeRuns(out))
}

// dedupeRuns collapses consecutive entries equal after Normalize into the
// first one suffixed with "(×N)", like compress.DedupeRuns.
func dedupeRuns(es []entry) []entry {
	var out []entry
	for i := 0; i < len(es); {
		j := i + 1
		norm := compress.Normalize(es[i].text)
		e := es[i]
		for j < len(es) && compress.Normalize(es[j].text) == norm {
			e.srcs = union(e.srcs, es[j].srcs)
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

// limit keeps warnings/errors plus head and tail when still too long.
func (o *Optimizer) limit(es []entry) []entry {
	if len(es) <= o.cfg.MaxLines {
		return es
	}
	head, tail := o.cfg.MaxLines/4, o.cfg.MaxLines/4
	var out []entry
	dropped := 0
	for i, e := range es {
		if i < head || i >= len(es)-tail || severity(e.text) != compress.None || continuation.MatchString(e.text) {
			if dropped > 0 {
				out = append(out, entry{text: compress.Omitted(dropped, "")})
				dropped = 0
			}
			out = append(out, e)
			continue
		}
		dropped++
	}
	return out
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
