package mcptool

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
)

// Slice picks part of an original, so reading back a detail costs less than
// the whole text. The zero value selects everything.
type Slice struct {
	Lines   string // "120-180", "120-" or "-40": a 1-based inclusive range, printed verbatim
	Grep    string // regexp: matching lines with their numbers
	Context int    // lines around each grep or query match
	Query   string // the lines best matching these words (BM25), with their numbers
	Max     int    // query: at most this many matching lines (default 40)
}

// Partial reports whether s selects less than the whole text.
func (s Slice) Partial() bool { return s.Lines != "" || s.Grep != "" || s.Query != "" }

// Apply returns the selected part of text.
func (s Slice) Apply(text string) (string, error) {
	if !s.Partial() {
		return text, nil
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if s.Lines != "" {
		from, to, err := lineRange(s.Lines, len(lines))
		if err != nil {
			return "", err
		}
		return strings.Join(lines[from:to], ""), nil
	}
	var hits []int
	if s.Grep != "" {
		re, err := regexp.Compile(s.Grep)
		if err != nil {
			return "", fmt.Errorf("grep: %w", err)
		}
		for i, l := range lines {
			if re.MatchString(l) {
				hits = append(hits, i)
			}
		}
	} else {
		hits = queryHits(lines, s.Query, cmp.Or(s.Max, 40))
	}
	if len(hits) == 0 {
		return "no matching lines\n", nil
	}
	return numbered(lines, hits, s.Context), nil
}

// lineRange parses "a-b", "a-", "-b" or "a" into a 0-based half-open range.
func lineRange(spec string, n int) (int, int, error) {
	a, b, isRange := strings.Cut(spec, "-")
	if !isRange {
		b = a
	}
	from, to := 1, n
	var err error
	if a != "" {
		if from, err = strconv.Atoi(a); err != nil || from < 1 {
			return 0, 0, fmt.Errorf("lines %q: want 120-180", spec)
		}
	}
	if b != "" {
		if to, err = strconv.Atoi(b); err != nil || to < from {
			return 0, 0, fmt.Errorf("lines %q: want 120-180", spec)
		}
	}
	if from > n {
		return 0, 0, fmt.Errorf("lines %q: text has %d lines", spec, n)
	}
	return from - 1, min(to, n), nil
}

// queryHits returns the indexes of up to k lines scoring at least a third of
// the best BM25 match, in text order.
func queryHits(lines []string, query string, k int) []int {
	docs := make([][]string, len(lines))
	for i, l := range lines {
		docs[i] = sample.Terms(l)
	}
	scores := sample.BM25(docs, sample.Terms(query))
	best := slices.Max(append(scores, 0))
	if best == 0 {
		return nil
	}
	var hits []int
	for i, sc := range scores {
		if sc >= best/3 {
			hits = append(hits, i)
		}
	}
	slices.SortStableFunc(hits, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })
	hits = hits[:min(k, len(hits))]
	slices.Sort(hits)
	return hits
}

// numbered prints the hit lines plus context as "N:text" (context "N-text"),
// separate runs split by "--", like grep -n.
func numbered(lines []string, hits []int, context int) string {
	isHit := map[int]bool{}
	keep := map[int]bool{}
	for _, h := range hits {
		isHit[h] = true
		for i := max(h-context, 0); i <= min(h+context, len(lines)-1); i++ {
			keep[i] = true
		}
	}
	var b strings.Builder
	last := -1
	for i, l := range lines {
		if !keep[i] {
			continue
		}
		if last >= 0 && i > last+1 {
			b.WriteString("--\n")
		}
		sep := "-"
		if isHit[i] {
			sep = ":"
		}
		fmt.Fprintf(&b, "%d%s%s", i+1, sep, strings.TrimSuffix(l, "\n"))
		b.WriteByte('\n')
		last = i
	}
	return b.String()
}
