package k8soptimizer

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	// resultLine is `kind/name verb [(dry run)]` or `kind "name" verb` (delete).
	resultLine = lazyre.New(`^([a-z][\w.-]*)(?:/\S+| "\S+") (unchanged|configured|created|deleted|pruned)\b`)
	waitLine   = lazyre.New(`^Waiting for .* (rollout to finish|to be ready)|^Waiting for deployment spec update`)
	rolledOut  = lazyre.New(`successfully rolled out$`)
)

const maxKinds = 6

// apply folds `unchanged` lines to one count with kinds; created, configured
// and deleted resources, warnings and errors stay as they came.
func apply(lines []string) []string {
	var out []string
	kinds := map[string]int{}
	at, total := -1, 0
	for _, l := range lines {
		m := resultLine.FindStringSubmatch(l)
		if m == nil || m[2] != "unchanged" {
			out = append(out, l)
			continue
		}
		if at < 0 {
			at = len(out)
			out = append(out, "")
		}
		kinds[m[1]]++
		total++
	}
	if at >= 0 {
		out[at] = unchangedSummary(kinds, total)
	}
	return out
}

// unchangedSummary reads `unchanged ×120 (deployment.apps ×50, service ×40, …)`.
func unchangedSummary(kinds map[string]int, total int) string {
	names := make([]string, 0, len(kinds))
	for k := range kinds {
		names = append(names, k)
	}
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Or(kinds[b]-kinds[a], strings.Compare(a, b)) // ties by name: deterministic
	})
	var parts []string
	for _, k := range names[:min(len(names), maxKinds)] {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, kinds[k]))
	}
	if len(names) > maxKinds {
		parts = append(parts, "…")
	}
	return fmt.Sprintf("unchanged ×%d (%s)", total, strings.Join(parts, ", "))
}

// rollout drops progress lines: a run followed by the success line becomes a
// count on it; a run that ends otherwise (timeout, error) keeps its last line.
func rollout(lines []string) []string {
	var out []string
	run := 0
	flush := func(last string) {
		if run > 1 {
			out = append(out, fmt.Sprintf("%s (×%d)", last, run))
		} else if run == 1 {
			out = append(out, last)
		}
		run = 0
	}
	var last string
	for _, l := range lines {
		if waitLine.MatchString(l) {
			run++
			last = l
			continue
		}
		if rolledOut.MatchString(l) && run > 0 {
			l += fmt.Sprintf(" (after %d progress lines)", run)
			run = 0
		}
		flush(last)
		out = append(out, l)
	}
	flush(last)
	return out
}
