package docoptimizer

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/tokens"
)

var heading = regexp.MustCompile(`^(#{1,6}) `)

// part is a heading and the lines up to the next one; the first section
// of a page is what comes before any heading and has level 0.
type part struct {
	level  int
	lines  []string
	tokens int
}

// sections fits markdown into a token budget by whole sections: the intro,
// then sections matching the query (best first), then the rest in page order
// while they fit. Dropped sections keep their heading when it is one of the
// page's top two levels, so the outline stays readable; the rest of what was
// dropped becomes "… N lines omitted".
func sections(lines []string, query string, budget int) []string {
	secs := split(lines)
	total := 0
	for _, s := range secs {
		total += s.tokens
	}
	if total <= budget {
		return lines
	}
	keep := make([]bool, len(secs))
	keep[0] = true
	used := secs[0].tokens
	for _, i := range byRelevance(secs, query) {
		if used+secs[i].tokens <= budget {
			keep[i] = true
			used += secs[i].tokens
		}
	}
	for i, s := range secs {
		if keep[i] {
			continue
		}
		if used+s.tokens > budget {
			break
		}
		keep[i] = true
		used += s.tokens
	}
	return render(secs, keep)
}

func split(lines []string) []*part {
	secs := []*part{{}}
	inCode := false
	for _, l := range lines {
		if fence.MatchString(l) {
			inCode = !inCode
		}
		if m := heading.FindStringSubmatch(l); m != nil && !inCode {
			secs = append(secs, &part{level: len(m[1])})
		}
		cur := secs[len(secs)-1]
		cur.lines = append(cur.lines, l)
		cur.tokens += tokens.Estimate(l) + 1
	}
	return secs
}

// byRelevance lists sections matching the query well (at least half the best
// score), best first: a page full of weak matches would otherwise fill the
// budget with them.
func byRelevance(secs []*part, query string) []int {
	terms := rows.Terms(query)
	if len(terms) == 0 {
		return nil
	}
	docs := make([][]string, len(secs))
	for i, s := range secs {
		docs[i] = rows.Terms(strings.Join(s.lines, "\n"))
	}
	scores := rows.BM25(docs, terms)
	best := slices.Max(scores)
	var hits []int
	for i := 1; i < len(secs); i++ {
		if scores[i] > 0 && scores[i] >= best/2 {
			hits = append(hits, i)
		}
	}
	slices.SortStableFunc(hits, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })
	return hits
}

func render(secs []*part, keep []bool) []string {
	top := 6
	for _, s := range secs[1:] {
		top = min(top, s.level)
	}
	var out []string
	omitted := 0
	flush := func() {
		if omitted > 0 {
			out = append(out, compress.Omitted(omitted, "lines"))
			omitted = 0
		}
	}
	for i, s := range secs {
		switch {
		case keep[i]:
			flush()
			out = append(out, s.lines...)
		case s.level <= top+1:
			flush()
			out = append(out, s.lines[0])
			omitted += len(s.lines) - 1
		default:
			omitted += len(s.lines)
		}
	}
	flush()
	return out
}
