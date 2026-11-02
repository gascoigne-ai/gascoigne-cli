package logoptimizer

import "math"

const (
	// burstMinRepeats is the fewest repeats worth saying where they sit.
	burstMinRepeats = 20
	// burstShare of the repeats must fall in the stretch...
	burstShare = 0.8
	// ...and the stretch must cover at most this share of the log.
	burstSpan = 0.25
)

// burst finds, among the sorted line positions of one template's n repeats,
// the shortest stretch holding burstShare of them. It reports that stretch
// (first and last line index) when it covers at most burstSpan of the total
// lines: repeats in a storm, not a steady heartbeat.
func burst(at []int, n, total int) (first, last int, ok bool) {
	if n < burstMinRepeats || total == 0 || len(at) != n {
		return 0, 0, false
	}
	k := int(math.Ceil(burstShare * float64(n)))
	best := -1
	for j := 0; j+k-1 < n; j++ {
		if span := at[j+k-1] - at[j]; best < 0 || span < best {
			best, first, last = span, at[j], at[j+k-1]
		}
	}
	if best < 0 || float64(best+1) > burstSpan*float64(total) {
		return 0, 0, false
	}
	return first, last, true
}
