package dboptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

func TestPlanRulesVerify(t *testing.T) {
	for name, rules := range map[string]interface{ Verify() error }{"planDetail": planDetail, "planNoise": planNoise} {
		if err := rules.Verify(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// The regexps planDetail and planNoise replaced, kept to pin the old behavior.
var (
	oldPlanDetail = lazyre.New(`^\s+(Filter|Index Cond|Hash Cond|Join Filter|Recheck Cond|Sort Key|Rows Removed by Filter|Group Key|Merge Cond|Output|Sort Method|Heap Fetches):`)
	oldPlanNoise  = lazyre.New(`^\s+(Buffers:|Worker \d+:|Planning:|I/O Timings:|Memory Usage:|Batches:)`)
)

func TestPlanNoiseSameAsRegexp(t *testing.T) {
	var ex []string
	for _, r := range planNoise {
		ex = append(ex, r.Yes...)
		ex = append(ex, r.No...)
	}
	ex = append(ex, "  Buffers: shared hit=3", "Buffers: shared hit=3", "    Worker 0:  Sort Method: quicksort", "  Worker 12: x", "  Worker :", "  Worker x:", "  Planning:", "  Planning Time: 0.1 ms", "Planning Time: 0.1 ms",
		"  I/O Timings: read=0.1", "  Memory Usage: 24kB", "  Batches: 1", "  Batches 1", "\tBuffers: x", "  Buffers", "  Planning:x")
	sameAsOld(t, oldPlanNoise.MatchString, planNoise.Match, ex)
}

func TestPlanDetailSameAsRegexp(t *testing.T) {
	var ex []string
	for _, r := range planDetail {
		ex = append(ex, r.Yes...)
		ex = append(ex, r.No...)
	}
	ex = append(ex, "  Filter: (a = 1)", "Filter: x", "  Filter (x)", "  Index Cond: (id = 1)", "  Hash Cond: (a.id = b.id)", "  Join Filter: x", "  Recheck Cond: x", "  Sort Key: a", "  Rows Removed by Filter: 5",
		"  Group Key: a", "  Merge Cond: x", "  Output: a, b", "  Sort Method: quicksort  Memory: 25kB", "  Heap Fetches: 0", "  Heap Fetches:", "  Filters: x", "\tFilter: x", "  Sort Keys: x", "  Output : x", "  Planning Time: 1 ms")
	sameAsOld(t, oldPlanDetail.MatchString, planDetail.Match, ex)
}
