package gitoptimizer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	fullSHA = lazyre.New(`\b[0-9a-f]{40}\b`)
	// worktreePad is the column padding of `git worktree list`.
	worktreePad = lazyre.New(`^(\S+) {2,}(\(bare\)|[0-9a-f]{7,40} .*)$`)
	// checkoutMove is a reflog entry for a branch switch.
	checkoutMove = lazyre.New(`^((?:[0-9a-f]{7,40} )?)(\S+)@\{(\d+)\}: checkout: moving from (.+) to (.+)$`)
)

// minFold is the shortest run of branch switches worth folding.
const minFold = 3

// refs shrinks ref listings (reflog, for-each-ref, submodule status, remote -v,
// worktree list): shas to 7, padding gone, fetch/push pairs of one URL merged,
// runs of branch switches folded.
func refs(lines []string) []string {
	lines = foldCheckouts(lines)
	var out []string
	for i := 0; i < len(lines); i++ {
		l := fullSHA.ReplaceAllStringFunc(lines[i], func(s string) string { return s[:7] })
		l = worktreePad.ReplaceAllString(l, "$1 $2")
		if fetch, ok := strings.CutSuffix(l, " (fetch)"); ok && i+1 < len(lines) && lines[i+1] == fetch+" (push)" {
			l = fetch + " (fetch, push)"
			i++
		}
		out = append(out, l)
	}
	return out
}

// foldCheckouts turns a run of "checkout: moving from A to B" entries into one
// line naming the branches; the ledger keeps the sequence.
func foldCheckouts(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i
		for j < len(lines) && checkoutMove.MatchString(lines[j]) {
			j++
		}
		if j-i < minFold {
			j = max(j, i+1)
			out = append(out, lines[i:j]...)
		} else {
			out = append(out, foldRun(lines[i:j]))
		}
		i = j
	}
	return out
}

func foldRun(run []string) string {
	first := checkoutMove.FindStringSubmatch(run[0])
	last := checkoutMove.FindStringSubmatch(run[len(run)-1])
	var names []string
	for _, l := range run {
		for _, n := range checkoutMove.FindStringSubmatch(l)[4:6] {
			if !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	more := ""
	if len(names) > 6 {
		more = fmt.Sprintf(" +%d", len(names)-6)
		names = names[:6]
	}
	return fmt.Sprintf("%s%s@{%s..%s}: checkout: %d moves between %s%s", first[1], first[2], first[3], last[3], len(run), strings.Join(names, ", "), more)
}
