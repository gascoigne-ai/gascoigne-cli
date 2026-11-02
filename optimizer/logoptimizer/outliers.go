package logoptimizer

import (
	"math"
	"slices"
	"strconv"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
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
func (o *Optimizer) outliers(lines, norms []string, vols [][]string, important []bool) []int {
	groups := map[string][]int{}
	for i := range lines {
		if !important[i] {
			groups[norms[i]] = append(groups[norms[i]], i)
		}
	}
	var out []int
	for _, idx := range groups {
		if len(idx) >= outlierMinLines {
			out = append(out, o.groupOutliers(lines, vols, idx)...)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// groupOutliers reads the volatile tokens of the lines at idx from vols, or
// from the lines when vols is nil (a custom normalizer is in use).
func (o *Optimizer) groupOutliers(lines []string, vols [][]string, idx []int) []int {
	tokens := make([][]string, len(idx))
	for j, i := range idx {
		if vols != nil {
			tokens[j] = vols[i]
		} else {
			tokens[j] = compress.Volatile(lines[i])
		}
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
		scale := 0.0
		if o.cfg.Outliers == outliersMAD {
			scale = spread(vals, median)
		}
		for j, v := range vals {
			if v >= outlierFactor*median || scale > 0 && v >= madMinRise*median && (v-median)/scale > madScore {
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

const (
	// madScore is the modified z-score (Iglewicz and Hoaglin) from which a
	// value stands out.
	madScore = 3.5
	// madMinRise keeps a value that is statistically far but barely above the
	// median (11 among 10s) from counting.
	madMinRise = 1.5
)

// spread is the scale of the usual variation around the median: MAD/0.6745,
// so that (v-median)/spread is the modified z-score of Iglewicz and Hoaglin.
// When most values are equal MAD is 0 and the mean absolute deviation stands
// in (the same paper's fallback, scaled by 1.253314). 0 means no spread.
func spread(vals []float64, median float64) float64 {
	dev := make([]float64, len(vals))
	sum := 0.0
	for i, x := range vals {
		dev[i] = math.Abs(x - median)
		sum += dev[i]
	}
	if mad := slices.Sorted(slices.Values(dev))[len(dev)/2]; mad > 0 {
		return mad / 0.6745
	}
	return 1.253314 * sum / float64(len(vals))
}
