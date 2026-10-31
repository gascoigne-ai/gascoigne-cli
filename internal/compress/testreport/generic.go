package testreport

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// decoration lines carry no text: ⎯⎯⎯⎯⎯[1/2]⎯⎯⎯, ─────, ===== (pytest headers have text).
var decoration = lazyre.New(`^[\s⎯─━═=_*~-]*(\[\d+/\d+\])?[\s⎯─━═=_*~-]*$`)

// Dialects lists runners Compress understands besides go.
func Dialects() []string {
	out := []string{"go", "go-json"}
	for d := range dialects {
		out = append(out, d)
	}
	return out
}

// Compress applies a runner's rules: passes counted and dropped, noise and
// dependency frames dropped, everything else (failures, diffs, summaries) kept.
func Compress(dialect string, lines []string) ([]string, bool) {
	r, ok := dialects[dialect]
	if !ok {
		return lines, false
	}
	var out []string
	passes, frames := 0, 0
	flush := func() {
		if frames > 0 {
			out = append(out, compress.Omitted(frames, "dependency stack frames"))
			frames = 0
		}
	}
	for _, l := range lines {
		switch {
		case r.keep.Match(l):
			flush()
			out = append(out, l)
		case r.pass.Match(l):
			passes++
		case r.frames.Match(l):
			frames++
		case r.noise.Match(l), strings.TrimSpace(l) != "" && decoration.MatchString(l):
		default:
			flush()
			out = append(out, l)
		}
	}
	flush()
	out = compress.SqueezeBlank(trimBlankEnds(out))
	if passes > 0 {
		out = append(out, compress.Omitted(passes, "passing test lines"))
	}
	return out, true
}

func trimBlankEnds(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
