// Package trace folds the waste in stack traces without dropping a frame
// kind: a recursion's frame cycle repeated hundreds of times is shown once
// with a count, and goroutines of a Go dump parked on the same stack are
// shown once with the ids of the others.
package trace

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Cycles folds runs where a block of up to maxPeriod lines repeats at least
// minRepeats times in a row: the first copy stays, a marker counts the rest.
func Cycles(lines []string, maxPeriod, minRepeats int) []string {
	var out []string
	for i := 0; i < len(lines); {
		period, reps := cycleAt(lines, i, maxPeriod)
		if reps < minRepeats {
			out = append(out, lines[i])
			i++
			continue
		}
		out = append(out, lines[i:i+period]...)
		out = append(out, fmt.Sprintf("… %s above repeated %d more times", plural(period, "line"), reps-1))
		i += period * reps
	}
	return out
}

// cycleAt finds the period at i repeated most often (by lines covered,
// shorter periods first), and how many times it occurs.
func cycleAt(lines []string, i, maxPeriod int) (period, reps int) {
	best := 0
	for p := 1; p <= maxPeriod && i+2*p <= len(lines); p++ {
		if strings.TrimSpace(lines[i]) == "" {
			break
		}
		n := 1
		for i+(n+1)*p <= len(lines) && slices.Equal(lines[i:i+p], lines[i+n*p:i+(n+1)*p]) {
			n++
		}
		if n > 1 && n*p > best {
			best, period, reps = n*p, p, n
		}
	}
	return period, reps
}

var (
	// goroutineHead is a Go dump's goroutine header: goroutine 37 [chan receive, 5 minutes]:
	goroutineHead = lazyre.New(`^goroutine (\d+) \[([^,\]]+)(, [^\]]+)?\]:$`)
	// goVolatile are what differs between goroutines on the same stack:
	// argument values, pointers and the parent goroutine's id.
	goVolatile = lazyre.New(`0x[0-9a-f]+\??|in goroutine \d+`)
)

// Goroutines groups the goroutines of a Go dump that wait in the same state
// on the same stack: the first one stays with its frames, the header names
// how many more there are and their ids. Groups smaller than min stay as
// they are.
func Goroutines(lines []string, min int) []string {
	type group struct {
		first int // index of the first member's header in blocks
		ids   []string
	}
	type block struct {
		lines []string
		key   string // "" for text outside goroutines
		id    string
	}
	var blocks []block
	for i := 0; i < len(lines); {
		m := goroutineHead.FindStringSubmatch(lines[i])
		if m == nil {
			blocks = append(blocks, block{lines: lines[i : i+1]})
			i++
			continue
		}
		j := i + 1
		for j < len(lines) && strings.TrimSpace(lines[j]) != "" && !goroutineHead.MatchString(lines[j]) {
			j++
		}
		frames := goVolatile.ReplaceAllString(strings.Join(lines[i+1:j], "\n"), "#")
		blocks = append(blocks, block{lines: lines[i:j], key: m[2] + "\n" + frames, id: m[1]})
		i = j
	}
	groups := map[string]*group{}
	for i, b := range blocks {
		if b.key == "" {
			continue
		}
		if g, ok := groups[b.key]; ok {
			g.ids = append(g.ids, b.id)
			continue
		}
		groups[b.key] = &group{first: i, ids: []string{b.id}}
	}
	var out []string
	for i, b := range blocks {
		g := groups[b.key]
		switch {
		case b.key == "" || len(g.ids) < min:
			out = append(out, b.lines...)
		case g.first == i:
			out = append(out, fmt.Sprintf("%s +%d more on this stack: %s", b.lines[0], len(g.ids)-1, ids(g.ids[1:])))
			out = append(out, b.lines[1:]...)
		default:
			// Folded into the group's first member: drop it and the blank
			// line after it.
			if i+1 < len(blocks) && blocks[i+1].key == "" && strings.TrimSpace(blocks[i+1].lines[0]) == "" {
				blocks[i+1].lines = nil
			}
		}
	}
	return out
}

// ids lists goroutine ids, the first 10 then a count.
func ids(list []string) string {
	if len(list) <= 10 {
		return strings.Join(list, " ")
	}
	return fmt.Sprintf("%s …%d more", strings.Join(list[:10], " "), len(list)-10)
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
