// Package terse drops low-information words from instruction prose
// (CLAUDE.md, skills, guides) that models read with every request: articles,
// fillers, hedges and politeness. Only whole words go — character tricks
// (vowel dropping, abbreviations) split one-token words into several. Words
// are scored by self-information, -log2 of their frequency, and the
// cheapest go first until a target share of the prose's words is dropped.
// Only listed words are candidates (never negations, modals, conjunctions
// or content words); code, paths, identifiers, quotes, headings, tables and
// fenced blocks always stay. Deterministic.
package terse

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"
)

// perMillion are approximate frequencies (occurrences per million words of
// general English) of the words that may be dropped: articles, fillers,
// hedges, intensifiers, politeness and discourse markers. Conjunctions,
// pronouns, prepositions and verbs carry an instruction's structure, so
// they are not here however frequent.
var perMillion = map[string]float64{
	"the": 56000, "a": 21600, "an": 3600,
	"very": 1000, "just": 1100, "really": 600, "basically": 60, "actually": 300, "simply": 120,
	"quite": 300, "rather": 260, "fairly": 60, "pretty": 140, "somewhat": 40, "essentially": 50,
	"generally": 90, "usually": 160, "typically": 50, "probably": 220, "perhaps": 230, "maybe": 120,
	"likely": 120, "possibly": 50, "please": 60, "kindly": 10, "certainly": 90, "definitely": 40,
	"obviously": 50, "clearly": 90, "indeed": 110, "furthermore": 30, "moreover": 25,
	"additionally": 15, "also": 1200, "currently": 60, "extremely": 50, "highly": 90,
}

// word is a candidate in the text: its byte span and score.
type word struct {
	from, to int
	bits     float64
}

// Prune drops up to ratio (0..1) of the prose's words, cheapest first.
func Prune(text string, ratio float64) string {
	lines := strings.SplitAfter(text, "\n")
	type cand struct {
		line int
		w    word
	}
	var cands []cand
	total := 0
	fenced := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "|") || t == "" {
			continue
		}
		words, n := candidates(l)
		total += n
		for _, w := range words {
			cands = append(cands, cand{i, w})
		}
	}
	budget := int(float64(total) * ratio)
	slices.SortStableFunc(cands, func(a, b cand) int { return cmp.Compare(a.w.bits, b.w.bits) })
	drop := map[int][]word{}
	for _, c := range cands[:min(budget, len(cands))] {
		drop[c.line] = append(drop[c.line], c.w)
	}
	var b strings.Builder
	for i, l := range lines {
		b.WriteString(remove(l, drop[i]))
	}
	return b.String()
}

// candidates lists the droppable words of a prose line and counts its words.
// Text inside backticks or quotes is never touched.
func candidates(l string) ([]word, int) {
	var out []word
	n := 0
	inCode, inQuote := false, false
	for i := 0; i < len(l); {
		c := l[i]
		switch {
		case c == '`':
			inCode = !inCode
			i++
			continue
		case c == '"' && !inCode:
			inQuote = !inQuote
			i++
			continue
		}
		r := rune(c)
		if r >= 0x80 || !unicode.IsLetter(r) {
			i++
			continue
		}
		j := i
		for j < len(l) && (l[j] < 0x80 && (unicode.IsLetter(rune(l[j])) || l[j] == '\'')) {
			j++
		}
		// A word glued to path, identifier or number characters is not prose.
		glued := i > 0 && strings.ContainsRune("/._-$@:=<>[]{}()0123456789", rune(l[i-1])) ||
			j < len(l) && strings.ContainsRune("/._-$@=<>[]{}(0123456789", rune(l[j]))
		w, lower := l[i:j], strings.ToLower(l[i:j])
		n++
		// Capitalized words may be names ("May", "Will"); articles starting
		// a sentence are not.
		cased := w == lower || lower == "the" || lower == "a" || lower == "an"
		f, listed := perMillion[lower]
		if listed && cased && !inCode && !inQuote && !glued {
			out = append(out, word{i, j, -math.Log2(f / 1e6)})
		}
		i = j
	}
	return out, n
}

// remove cuts the words (and one space next to each) from l.
func remove(l string, ws []word) string {
	if len(ws) == 0 {
		return l
	}
	slices.SortFunc(ws, func(a, b word) int { return b.from - a.from })
	for _, w := range ws {
		from, to := w.from, w.to
		if to < len(l) && l[to] == ' ' {
			to++
		} else if from > 0 && l[from-1] == ' ' {
			from--
		}
		l = l[:from] + l[to:]
	}
	return l
}
