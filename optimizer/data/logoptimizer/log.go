// Package logoptimizer compresses logs: warnings and errors stay with their
// context and stack traces, debug noise goes, other lines collapse to one
// example per template with a count. Plain text only loses repeated lines.
package logoptimizer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		lines := compress.Lines(compress.StripANSI(b.Text))
		if b.Type == detect.Text {
			return compress.Join(compress.SqueezeBlank(dedupeExact(lines))), nil
		}
		return compress.Join(o.log(lines)), nil
	}, detect.Log, detect.Text)
}

var (
	levelRe = regexp.MustCompile(`\b(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERROR|ERR|FATAL|CRIT|CRITICAL|PANIC)\b|\blevel=(\w+)`)
	// Continuation lines belong to the entry above: stack frames, wrapped text.
	continuation = regexp.MustCompile(`^(\s+|at |Caused by|Traceback|\.\.\. \d+ more)`)
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
	for i, l := range lines {
		if !important[i] {
			counts[compress.Normalize(l)]++
		}
	}
	var out []string
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
			out = append(out, compress.Omitted(dropped, ""))
			dropped = 0
		}
		if !important[i] {
			seen[norm] = true
			if n := counts[norm]; n > 1 {
				l = fmt.Sprintf("%s (×%d)", l, n)
			}
		}
		out = append(out, l)
	}
	if dropped > 0 {
		out = append(out, compress.Omitted(dropped, ""))
	}
	return o.limit(compress.DedupeRuns(out))
}

// limit keeps warnings/errors plus head and tail when still too long.
func (o *Optimizer) limit(lines []string) []string {
	if len(lines) <= o.cfg.MaxLines {
		return lines
	}
	head, tail := o.cfg.MaxLines/4, o.cfg.MaxLines/4
	var out []string
	dropped := 0
	for i, l := range lines {
		if i < head || i >= len(lines)-tail || severity(l) != compress.None || continuation.MatchString(l) {
			if dropped > 0 {
				out = append(out, compress.Omitted(dropped, ""))
				dropped = 0
			}
			out = append(out, l)
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
