// Package compress holds deterministic text primitives shared by optimizers.
// Everything here is lossy on purpose; core keeps originals retrievable.
package compress

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var ansi = lazyre.New(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\r`)

func StripANSI(s string) string { return ansi.ReplaceAllString(s, "") }

// Lines splits s into lines, dropping the empty element after a final newline.
func Lines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Join joins lines and ends with a newline when there is any line.
func Join(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// Omitted is the marker left where lines were dropped.
func Omitted(n int, what string) string {
	if what == "" {
		what = "lines"
	}
	if n == 1 {
		what = singular(what)
	}
	return fmt.Sprintf("… %d %s omitted", n, what)
}

var volatile = lazyre.New(
	`\d{4}-\d\d-\d\d[T ]\d\d:\d\d:\d\d(\.\d+)?(Z|[+-]\d\d:?\d\d)?` + // timestamps
		`|\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b` + // uuids
		`|\b0x[0-9a-fA-F]+\b|\b[0-9a-f]{8,}\b` + // hex ids, hashes (task ids, short shas)
		`|\d+(\.\d+)?`) // numbers

// Normalize replaces volatile tokens (timestamps, ids, numbers) so lines that
// differ only by them compare equal.
func Normalize(line string) string {
	return volatile.ReplaceAllString(line, "#")
}

// Volatile returns the tokens Normalize replaces, in order: lines with the
// same Normalize form have them at the same places.
func Volatile(line string) []string {
	return volatile.FindAllString(line, -1)
}

// DedupeRuns collapses consecutive lines equal after Normalize into the first
// one suffixed with "(×N)".
func DedupeRuns(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i + 1
		norm := Normalize(lines[i])
		for j < len(lines) && Normalize(lines[j]) == norm {
			j++
		}
		if n := j - i; n > 1 && strings.TrimSpace(lines[i]) != "" { // blank runs: see SqueezeBlank
			out = append(out, fmt.Sprintf("%s (×%d)", lines[i], n))
		} else {
			out = append(out, lines[i])
		}
		i = j
	}
	return out
}

// DedupeAll keeps the first occurrence of every normalized line, annotated with
// its total count when repeated. Order of first occurrences is preserved.
func DedupeAll(lines []string) []string {
	count := map[string]int{}
	for _, l := range lines {
		count[Normalize(l)]++
	}
	seen := map[string]bool{}
	var out []string
	for _, l := range lines {
		norm := Normalize(l)
		if seen[norm] {
			continue
		}
		seen[norm] = true
		if n := count[norm]; n > 1 {
			l = fmt.Sprintf("%s (×%d)", l, n)
		}
		out = append(out, l)
	}
	return out
}

// KeepWithContext keeps lines where keep is true plus before/after lines of
// context; dropped runs become an Omitted marker.
func KeepWithContext(lines []string, keep func(string) bool, before, after int) []string {
	marked := make([]bool, len(lines))
	for i, l := range lines {
		if !keep(l) {
			continue
		}
		for j := max(0, i-before); j <= min(len(lines)-1, i+after); j++ {
			marked[j] = true
		}
	}
	var out []string
	dropped := 0
	for i, l := range lines {
		if !marked[i] {
			dropped++
			continue
		}
		if dropped > 0 {
			out = append(out, Omitted(dropped, ""))
			dropped = 0
		}
		out = append(out, l)
	}
	if dropped > 0 {
		out = append(out, Omitted(dropped, ""))
	}
	return out
}

// HeadTail keeps the first head and last tail lines.
func HeadTail(lines []string, head, tail int) []string {
	if len(lines) <= head+tail+1 {
		return lines
	}
	out := append([]string{}, lines[:head]...)
	out = append(out, Omitted(len(lines)-head-tail, ""))
	return append(out, lines[len(lines)-tail:]...)
}

// TruncateLine shortens s to about max runes, cutting the middle.
func TruncateLine(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	half := max / 2
	return string(r[:half]) + " … " + string(r[len(r)-half:])
}

// SqueezeBlank collapses runs of blank lines into one and trims trailing spaces.
func SqueezeBlank(lines []string) []string {
	var out []string
	blank := false
	for _, l := range lines {
		l = strings.TrimRight(l, " \t")
		if l == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, l)
	}
	return out
}

type Severity int

const (
	None Severity = iota
	Warn
	Error
)

var (
	errorRe = lazyre.New(`(?i)\berr!|\b(errors?|fail(ed|ure|ing)?|fatal|panic|exception|traceback|segfault|denied|refused|timed? ?out|cannot|can't|unable|undefined|not found)\b`)
	warnRe  = lazyre.New(`(?i)\b(warn(ing)?|deprecated)\b`)
)

// SeverityOf classifies a line by error/warning vocabulary.
func SeverityOf(line string) Severity {
	switch {
	case errorRe.MatchString(line):
		return Error
	case warnRe.MatchString(line):
		return Warn
	}
	return None
}

func IsError(line string) bool { return SeverityOf(line) == Error }

// singular drops the plural ending: items→item, matches→match, nodes→node.
func singular(s string) string {
	for _, suf := range []string{"ches", "shes", "sses", "xes"} {
		if strings.HasSuffix(s, suf) {
			return s[:len(s)-2]
		}
	}
	return strings.TrimSuffix(s, "s")
}
