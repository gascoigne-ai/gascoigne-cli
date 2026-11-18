package compress

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// ByteStyle spells the units of HumanBytes: Units from bytes up (the last one
// has no larger neighbour), Sep between number and unit, Whole to cut the
// decimal instead of printing one.
type ByteStyle struct {
	Units []string
	Sep   string
	Whole bool
}

var (
	// LongBytes prints "12 B", "1.5 KB", "3 GB".
	LongBytes = ByteStyle{Units: []string{"B", "KB", "MB", "GB", "TB"}, Sep: " "}
	// ShortBytes prints "12", "1.5K", "3G", like ls -h.
	ShortBytes = ByteStyle{Units: []string{"", "K", "M", "G", "T"}}
	// WholeKB prints "12 B", "46 KB" (floored, never MB): the notebook image
	// sizes, kept apart because its eval golden and a "[image/png 46 KB]" fact
	// pin that spelling; LongBytes there prints "46.2 KB" and eval regresses.
	WholeKB = ByteStyle{Units: []string{"B", "KB"}, Sep: " ", Whole: true}
)

// HumanBytes prints size in binary units, one decimal, a trailing ".0" dropped.
func HumanBytes(size int64, style ByteStyle) string {
	f := float64(size)
	for i, unit := range style.Units {
		if f < 1024 || i == len(style.Units)-1 {
			if i == 0 {
				return fmt.Sprintf("%d", size) + sep(style, unit) + unit
			}
			if style.Whole {
				return fmt.Sprintf("%d", int64(f)) + sep(style, unit) + unit
			}
			return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0") + sep(style, unit) + unit
		}
		f /= 1024
	}
	return ""
}

func sep(style ByteStyle, unit string) string {
	if unit == "" {
		return ""
	}
	return style.Sep
}

// Tally counts keys; it renders in a stable order.
type Tally map[string]int

// Counts renders "2xx 120, 5xx 3", by key or by descending count (ties by key).
func (t Tally) Counts(byCount bool) string {
	keys := slices.Sorted(maps.Keys(t))
	if byCount {
		slices.SortStableFunc(keys, func(left, right string) int { return cmp.Compare(t[right], t[left]) })
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, t[k])
	}
	return strings.Join(parts, ", ")
}

// Top renders the limit most frequent keys as "key ×n", then "… N more".
func (t Tally) Top(limit int) string {
	keys := slices.SortedFunc(maps.Keys(t), func(left, right string) int {
		return cmp.Or(cmp.Compare(t[right], t[left]), cmp.Compare(left, right))
	})
	var parts []string
	for _, k := range keys[:min(limit, len(keys))] {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, t[k]))
	}
	if len(keys) > limit {
		parts = append(parts, fmt.Sprintf("… %d more", len(keys)-limit))
	}
	return strings.Join(parts, ", ")
}

// Indent prefixes every line with two spaces.
func Indent(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = "  " + l
	}
	return out
}

// IsDigit reports an ASCII digit.
func IsDigit(char byte) bool { return char >= '0' && char <= '9' }

var htmlComment = lazyre.New(`(?s)<!--.*?-->`)

// StripHTMLComments drops <!-- ... --> comments (also multi-line ones).
func StripHTMLComments(text string) string { return htmlComment.ReplaceAllString(text, "") }

// DedupeExact collapses runs of three or more identical non-empty lines into
// the first one suffixed with "(×N)"; shorter runs and blank lines stay, so
// text keeps every distinct line.
func DedupeExact(lines []string) []string {
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
