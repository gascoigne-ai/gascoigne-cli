package detect

import (
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

const (
	sampleLines = 200
	sampleBytes = 64 << 10
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)

// sniffer recognizes a type from full text and sampled, ANSI-stripped lines.
type sniffer struct {
	typ   string
	match func(text string, lines []string) bool
}

// Order matters: specific formats before generic ones (log, code, text).
var sniffers = []sniffer{
	{GoTestJSON, func(_ string, l []string) bool {
		return len(l) > 0 && strings.Contains(l[0], `"Action":`) && strings.Contains(l[0], `"Package":`)
	}},
	{NDJSON, isNDJSON}, // before JSON: every NDJSON text is also a valid JSON stream
	{JSON, isJSON},
	{HTML, func(t string, _ []string) bool {
		head := strings.ToLower(t[:min(len(t), 2048)])
		return strings.Contains(head, "<!doctype html") || strings.Contains(head, "<html") ||
			strings.Contains(head, "<body") || strings.Contains(head, "<head")
	}},
	{Diff, anyLine(`^diff --git |^@@ -\d+(,\d+)? \+\d+(,\d+)? @@`)},
	{GitLog, anyLine(`^commit [0-9a-f]{40}`)},
	{GitStatus, anyLine(`^On branch |^HEAD detached |^Changes (not staged|to be committed)|^Untracked files:|^nothing to commit`)},
	{GoTest, anyLine(`^=== RUN |^--- (FAIL|PASS|SKIP): |^ok  \t|^FAIL\t|^\?   \t.*\[no test files\]`)},
	{GoBuild, anyLine(`^# [\w./-]+$|^\S+\.go:\d+:\d+: `)},
	{TSC, anyLine(`\.tsx?\(\d+,\d+\): error TS\d+|\.tsx?:\d+:\d+ - error TS\d+`)},
	{Bun, anyLine(`^bun (test|install|add|run) v\d`)},
	{NX, anyLine(`^\s*> nx run |NX\s+(Successfully ran|Ran target|Running target)`)},
	{JSTest, anyLine(`^\s*(PASS|FAIL)\s+\S+\.(test|spec)\.\w+|^Tests:\s+\d|^\s*Test Files\s+\d`)},
	{NPM, anyLine(`^npm (warn|WARN|ERR!|error|notice) |^added \d+ packages?|^up to date, audited|^> \S+@\S+ \S+`)},
	{Grep, ratio(`^[^\s:\d][^\s:]*:\d+[:-]`, 0.6)}, // path can't start with a digit: timestamps
	{LS, func(t string, l []string) bool { return lsTotal(t, l) && lsMode(t, l) }},
	{Log, ratio(`^\[?(\d{4}-\d\d-\d\d[T ]\d\d:\d\d|\d\d:\d\d:\d\d|[A-Z][a-z]{2} [ \d]\d \d\d:\d\d)|^\[?(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\b|\blevel=(debug|info|warn|error)`, 0.5)},
	{Code, func(t string, _ []string) bool { return Lang(t, Hints{}) != "" }},
}

var (
	lsTotal = anyLine(`^total \d+`)
	lsMode  = ratio(`^[-dlcbps][rwxsStT-]{9}[.+@]?\s`, 0.5)
)

// Sniff detects the content type from text alone.
func Sniff(text string) string {
	if strings.TrimSpace(text) == "" {
		return Text
	}
	// Full text is passed on (JSON validity needs all of it); lines are sampled.
	sample := ansi.ReplaceAllString(text[:min(len(text), sampleBytes)], "")
	lines := strings.Split(sample, "\n")
	lines = lines[:min(len(lines), sampleLines)]
	for _, s := range sniffers {
		if s.match(text, lines) {
			return s.typ
		}
	}
	return Text
}

func anyLine(pattern string) func(string, []string) bool {
	re := regexp.MustCompile(pattern)
	return func(_ string, lines []string) bool {
		for _, l := range lines {
			if re.MatchString(l) {
				return true
			}
		}
		return false
	}
}

// ratio matches when at least share of non-blank lines match pattern.
func ratio(pattern string, share float64) func(string, []string) bool {
	re := regexp.MustCompile(pattern)
	return func(_ string, lines []string) bool {
		n, hit := 0, 0
		for _, l := range lines {
			if strings.TrimSpace(l) == "" {
				continue
			}
			n++
			if re.MatchString(l) {
				hit++
			}
		}
		return n >= 2 && float64(hit) >= share*float64(n)
	}
}

// isJSON accepts one JSON value or a stream of concatenated values (go list -json).
func isJSON(t string, _ []string) bool {
	t = strings.TrimSpace(t)
	if !strings.HasPrefix(t, "{") && !strings.HasPrefix(t, "[") {
		return false
	}
	if json.Valid([]byte(t)) {
		return true
	}
	dec := json.NewDecoder(strings.NewReader(t))
	for {
		var v json.RawMessage
		if err := dec.Decode(&v); err == io.EOF {
			return true
		} else if err != nil {
			return false
		}
	}
}

func isNDJSON(_ string, lines []string) bool {
	n := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if !strings.HasPrefix(l, "{") || !json.Valid([]byte(l)) {
			return false
		}
		n++
	}
	return n >= 2
}
