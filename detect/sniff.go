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

// Sniff detects the type from text alone, trying specs by priority.
func Sniff(text string) string {
	if strings.TrimSpace(text) == "" {
		return Text
	}
	// Full text is passed on (JSON validity needs all of it); lines are sampled.
	sample := ansi.ReplaceAllString(text[:min(len(text), sampleBytes)], "")
	lines := strings.Split(sample, "\n")
	lines = lines[:min(len(lines), sampleLines)]
	for _, s := range specs {
		if s.Sniff != nil && s.Sniff(text, lines) {
			return s.Type
		}
	}
	return Text
}

// anyLine matches when some sampled line matches pattern.
func anyLine(pattern string) Sniffer {
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
func ratio(pattern string, share float64) Sniffer {
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

// all matches when every sniffer matches.
func all(sniffers ...Sniffer) Sniffer {
	return func(text string, lines []string) bool {
		for _, s := range sniffers {
			if !s(text, lines) {
				return false
			}
		}
		return true
	}
}

// firstLine matches the first non-blank line.
func firstLine(pattern string) Sniffer {
	re := regexp.MustCompile(pattern)
	return func(_ string, lines []string) bool {
		for _, l := range lines {
			if strings.TrimSpace(l) != "" {
				return re.MatchString(l)
			}
		}
		return false
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
