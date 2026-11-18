package gitoptimizer

import (
	"fmt"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// modeRunMin is the shortest run of same-mode create/delete lines that folds.
const modeRunMin = 3

var modeLine = lazyre.New(`^ (create|delete) mode (\d+) [^×].*$`)

// stat compresses the output of merge, rebase, cherry-pick, commit, checkout
// and stash pop: a --stat block loses its bars, runs of create/delete mode
// lines fold to a count. Everything else (conflicts, hints, errors) is kept.
func (o *Optimizer) stat(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		if n := statEnd(lines[i:]); n > 1 {
			out = append(out, statLines(lines[i:i+n])...)
			i += n
			continue
		}
		m := modeLine.FindStringSubmatch(lines[i])
		if m == nil {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i + 1
		for j < len(lines) {
			n := modeLine.FindStringSubmatch(lines[j])
			if n == nil || n[1] != m[1] || n[2] != m[2] {
				break
			}
			j++
		}
		if j-i >= modeRunMin {
			out = append(out, fmt.Sprintf(" %s mode %s ×%d", m[1], m[2], j-i))
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	return out
}
