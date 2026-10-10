package rows

import (
	"encoding/json"
	"math"
	"slices"
	"strings"
	"unicode"

	"github.com/lukaszb/gascoigne/lazyre"
)

// BM25 parameters, the usual defaults.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// dotted matches versions and other dotted words: go1.27.2, v2.3, 1.24.
var dotted = lazyre.New(`[\p{L}\p{N}]+(\.[\p{L}\p{N}]+)+`)

// Terms lowercases and splits text into alphanumeric terms (query or document).
// Versions are terms too, with their shorter prefixes: "go1.27.2" adds
// "go1.27.2" and "go1.27", so a query for go1.27 finds its patch releases
// instead of every line that mentions go1.
func Terms(s string) []string {
	s = strings.ToLower(s)
	terms := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, v := range dotted.FindAllString(s, -1) {
		if !strings.ContainsFunc(v, unicode.IsDigit) {
			continue
		}
		parts := strings.Split(v, ".")
		for n := len(parts); n >= 2; n-- {
			terms = append(terms, strings.Join(parts[:n], "."))
		}
	}
	return terms
}

// relevance scores items against query terms with BM25 and returns the top k
// (score > 0), scores normalized to 0..1.
func relevance(items []any, uniq []int, query []string, k int) map[int]float64 {
	if len(query) == 0 || k == 0 {
		return nil
	}
	docs := make([][]string, len(uniq))
	for d, i := range uniq {
		docs[d] = Terms(flatten(items[i]))
	}
	scores := BM25(docs, query)
	best := slices.Max(scores)
	out := map[int]float64{}
	for range k {
		top := -1
		for d, s := range scores {
			if s > 0 && (top < 0 || s > scores[top]) {
				top = d
			}
		}
		if top < 0 {
			break
		}
		out[uniq[top]] = scores[top] / best
		scores[top] = 0
	}
	return out
}

// BM25 scores each document (its terms) against the query terms; 0 is no match.
func BM25(docs [][]string, query []string) []float64 {
	scores := make([]float64, len(docs))
	if len(docs) == 0 || len(query) == 0 {
		return scores
	}
	df := map[string]int{}
	total := 0
	for _, doc := range docs {
		total += len(doc)
		seen := map[string]bool{}
		for _, t := range doc {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	avg := max(float64(total)/float64(len(docs)), 1)
	n := float64(len(docs))
	for d, doc := range docs {
		tf := map[string]int{}
		for _, t := range doc {
			tf[t]++
		}
		for _, q := range query {
			f := float64(tf[q])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(df[q])+0.5)/(float64(df[q])+0.5))
			scores[d] += idf * f * (bm25K1 + 1) / (f + bm25K1*(1-bm25B+bm25B*float64(len(doc))/avg))
		}
	}
	return scores
}

// flatten joins an item's keys and scalar values into one text.
func flatten(v any) string {
	var b strings.Builder
	var walk func(any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				b.WriteString(k + " ")
				walk(x)
			}
		case []any:
			for _, x := range v {
				walk(x)
			}
		case string:
			b.WriteString(v + " ")
		case json.Number:
			b.WriteString(v.String() + " ")
		}
	}
	walk(v)
	return b.String()
}
