package logoptimizer

import (
	"slices"
	"strconv"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
)

const (
	// outlierMinLines is the fewest lines of a template that tell what is usual.
	outlierMinLines = 8
	// outlierFactor is how far above the template's median a number must be.
	outlierFactor = 10
)

// outliers returns the lines, among those not already important, whose
// number at some place is far above the median of their template's lines
// there: one slow request among fast ones would otherwise fold into a ×N.
func outliers(lines []string, important []bool) []int {
	groups := map[string][]int{}
	for i, l := range lines {
		if !important[i] {
			norm := compress.Normalize(l)
			groups[norm] = append(groups[norm], i)
		}
	}
	var out []int
	for _, idx := range groups {
		if len(idx) >= outlierMinLines {
			out = append(out, groupOutliers(lines, idx)...)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func groupOutliers(lines []string, idx []int) []int {
	tokens := make([][]string, len(idx))
	for j, i := range idx {
		tokens[j] = compress.Volatile(lines[i])
	}
	var out []int
	for k := range tokens[0] {
		vals, ok := numbers(tokens, k)
		if !ok {
			continue
		}
		sorted := slices.Sorted(slices.Values(vals))
		median := sorted[len(sorted)/2]
		if median <= 0 {
			continue
		}
		for j, v := range vals {
			if v >= outlierFactor*median {
				out = append(out, idx[j])
			}
		}
	}
	return out
}

// numbers reads place k of every line as a number; timestamps, ids and
// hashes are not numbers and make the place unusable.
func numbers(tokens [][]string, k int) ([]float64, bool) {
	vals := make([]float64, len(tokens))
	for j, t := range tokens {
		if k >= len(t) {
			return nil, false
		}
		v, err := strconv.ParseFloat(t[k], 64)
		if err != nil {
			return nil, false
		}
		vals[j] = v
	}
	return vals, true
}
