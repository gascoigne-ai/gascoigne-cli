package docoptimizer

import (
	"cmp"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/tokens"
)

var heading = lazyre.New(`^(#{1,6}) `)

// part is a heading and the lines up to the next one; the first section
// of a page is what comes before any heading and has level 0.
type part struct {
	level  int
	lines  []string
	tokens int
}

// sections fits markdown into a token budget by whole sections: the intro,
// then sections matching the query (best first), then the rest in page order
// where they fit. A best match too big for what is left keeps its head, so a
// page that is one huge section still answers the query. Dropped sections
// keep their heading when it is one of the page's top two levels, so the
// outline stays readable; the rest of what was dropped becomes
// "… N lines omitted".
func sections(lines []string, query string, budget int) []string {
	secs := split(lines)
	total := 0
	for _, s := range secs {
		total += s.tokens
	}
	if total <= budget {
		return lines
	}
	keep := make([]int, len(secs)) // lines kept per section
	take := func(i int) {
		keep[i] = len(secs[i].lines)
	}
	take(0)
	used := secs[0].tokens
	for n, i := range byRelevance(secs, query) {
		switch {
		case used+secs[i].tokens <= budget:
			take(i)
			used += secs[i].tokens
		case n == 0:
			keep[i] = head(secs[i], budget-used)
			used = budget
		}
	}
	for i, s := range secs {
		if keep[i] == 0 && used+s.tokens <= budget {
			take(i)
			used += s.tokens
		}
	}
	return render(secs, keep)
}

// head is how many leading lines of s fit in budget tokens (at least the heading).
func head(s *part, budget int) int {
	used := 0
	for n, l := range s.lines {
		if used += tokens.Estimate(l) + 1; used > budget {
			return max(n, 1)
		}
	}
	return len(s.lines)
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

func render(secs []*part, keep []int) []string {
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
		case len(s.lines) == 0: // no intro before the first heading
		case keep[i] > 0:
			flush()
			out = append(out, s.lines[:keep[i]]...)
			omitted += len(s.lines) - keep[i]
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
