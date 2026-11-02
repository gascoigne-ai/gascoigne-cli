package sample

import (
	"cmp"
	"slices"
)

// TopN returns the indexes of the n best positive scores, best first; ties
// go to the earlier index.
func TopN(scores []float64, n int) []int {
	idx := make([]int, 0, len(scores))
	for i, s := range scores {
		if s > 0 {
			idx = append(idx, i)
		}
	}
	slices.SortStableFunc(idx, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })
	return idx[:min(n, len(idx))]
}

// MMR picks n indexes with positive scores by maximal marginal relevance
// (Carbonell and Goldstein, 1998): each pick maximizes lambda*score minus
// (1-lambda) times its Jaccard similarity to the closest line already picked,
// scores scaled to 0..1. Where TopN fills the budget with one line repeated
// under other numbers, MMR moves on to the next distinct match. lambda 1 is
// TopN. Ties go to the earlier index.
func MMR(scores []float64, docs [][]string, n int, lambda float64) []int {
	cands := TopN(scores, len(scores))
	if len(cands) == 0 || n <= 0 {
		return nil
	}
	top := scores[cands[0]]
	sets := make(map[int]map[string]bool, len(cands))
	for _, i := range cands {
		sets[i] = termSet(docs[i])
	}
	nearest := make(map[int]float64, len(cands)) // similarity to the closest pick
	var picked []int
	for len(picked) < n && len(cands) > 0 {
		best, bestVal := 0, -2.0
		for c, i := range cands {
			if v := lambda*scores[i]/top - (1-lambda)*nearest[i]; v > bestVal {
				best, bestVal = c, v
			}
		}
		pick := cands[best]
		cands = slices.Delete(cands, best, best+1)
		picked = append(picked, pick)
		for _, i := range cands {
			nearest[i] = max(nearest[i], jaccard(sets[i], sets[pick]))
		}
	}
	return picked
}

func termSet(terms []string) map[string]bool {
	s := make(map[string]bool, len(terms))
	for _, t := range terms {
		s[t] = true
	}
	return s
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	both := 0
	for t := range a {
		if b[t] {
			both++
		}
	}
	return float64(both) / float64(len(a)+len(b)-both)
}
