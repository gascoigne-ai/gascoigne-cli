// Package logoptimizer is the log family engine: warnings and errors stay
// with their context and stack traces, debug noise goes, other lines collapse
// to one example per template with a count. Structured cloud log events
// become one line each first; access logs become a traffic summary. Plain
// text only loses repeated lines.
package logoptimizer

import (
	"context"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Optimizer is the log family engine. Warnings and errors stay, and the other lines collapse
// to one example per template with a count.
type Optimizer struct {
	component.Nop
	cfg *Config
	// norm maps a line to its template; nil means compress.Normalize.
	norm func(string) string
}

func (o *Optimizer) normalize(line string) string {
	if o.norm == nil {
		return compress.Normalize(protectCodes(line))
	}
	return o.norm(line)
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

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	for i, b := range data.Blocks {
		if b.Type != detect.Text && detect.Family(b.Type) != detect.FamilyLog {
			continue
		}
		data.Blocks[i].Text = compress.Join(o.blockLines(b, compress.CleanLines(b.Text)))
	}
	return nil
}

// blockLines optimizes the lines of one text or log block by its type.
func (o *Optimizer) blockLines(block payload.Block, lines []string) []string {
	switch block.Type {
	case detect.Text:
		return compress.SqueezeBlank(o.foldText(compress.DedupeExact(lines)))
	case detect.Access:
		return o.access(lines)
	}
	for _, name := range o.cfg.Passes {
		if name != "prefixes" {
			lines = prePasses[name](block, lines)
		}
	}
	if s, ok := split(lines); ok && slices.Contains(o.cfg.Passes, "prefixes") {
		return render(o.template(s.lines, s.src), s.names)
	}
	return o.log(lines)
}

// prePasses are the stages that rewrite a log's lines before templating,
// by the names in Config.Passes ("prefixes" is handled where it renders).
var prePasses = map[string]func(block payload.Block, lines []string) []string{
	// Cloud log services and JSON loggers print structured events; gcloud
	// prints YAML.
	"entries": func(block payload.Block, lines []string) []string {
		text := strings.TrimSpace(block.Text)
		if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") || block.Type == detect.GCPLogs {
			if ev, ok := entries(text); ok {
				return ev
			}
		}
		return lines
	},
	"units": func(block payload.Block, lines []string) []string {
		if block.Type == detect.Journal || block.Type == detect.Syslog {
			return unitPairs(lines)
		}
		return lines
	},
}

var (
	levelRe = lazyre.New(`\b(TRACE|DEBUG|INFO|NOTICE|WARN|WARNING|ERROR|ERR|FATAL|CRIT|CRITICAL|PANIC)\b|\blevel=(\w+)`)
	// Continuation lines belong to the entry above: stack frames, wrapped text.
	continuation = lazyre.New(`^(\s+|at |Caused by|Traceback|\.\.\. \d+ more)`)
)

func severity(line string) compress.Severity {
	m := levelRe.FindStringSubmatch(line)
	if m == nil {
		return compress.SeverityOf(line)
	}
	switch strings.ToUpper(m[1] + m[2]) {
	case "WARN", "WARNING":
		return compress.Warn
	case "ERROR", "ERR", "FATAL", "CRIT", "CRITICAL", "PANIC":
		return compress.Error
	}
	return compress.None
}

func isDebug(line string) bool {
	m := levelRe.FindStringSubmatch(line)
	if m == nil {
		return false
	}
	lvl := strings.ToUpper(m[1] + m[2])
	return lvl == "TRACE" || lvl == "DEBUG"
}

func (o *Optimizer) log(lines []string) []string {
	return texts(o.template(lines, nil))
}
