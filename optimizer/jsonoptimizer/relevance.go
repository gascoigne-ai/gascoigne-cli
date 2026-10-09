package jsonoptimizer

import (
	"encoding/json"
	"math"
	"strings"
	"unicode"
)

// BM25 parameters, the usual defaults.
const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// terms lowercases and splits text into alphanumeric terms.
func terms(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// relevance scores items against query terms with BM25 and returns the top k
// (score > 0), scores normalized to 0..1.
func relevance(items []any, uniq []int, query []string, k int) map[int]float64 {
	if len(query) == 0 || k == 0 {
		return nil
	}
	docs := make([][]string, len(uniq))
	df := map[string]int{}
	total := 0
	for d, i := range uniq {
		docs[d] = terms(flatten(items[i]))
		total += len(docs[d])
		seen := map[string]bool{}
		for _, t := range docs[d] {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	avg := float64(total) / float64(len(docs))
	n := float64(len(docs))
	scores := make([]float64, len(docs))
	best := 0.0
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
		best = max(best, scores[d])
	}
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
