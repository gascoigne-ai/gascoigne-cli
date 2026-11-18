package logoptimizer

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

// dedupeRuns collapses consecutive entries equal after Normalize into the
// first one suffixed with "(×N)", like compress.DedupeRuns.
func (o *Optimizer) dedupeRuns(es []entry) []entry {
	norms := make([]string, len(es))
	for i, e := range es {
		norms[i] = o.normalize(e.text)
	}
	var out []entry
	for i := 0; i < len(es); {
		j := i + 1
		e := es[i]
		for j < len(es) && norms[j] == norms[i] && !e.solo && !es[j].solo {
			e.srcs = union(e.srcs, es[j].srcs)
			e.n += es[j].n
			j++
		}
		if n := j - i; n > 1 && strings.TrimSpace(e.text) != "" {
			e.text = fmt.Sprintf("%s (×%d)", e.text, n)
		}
		out = append(out, e)
		i = j
	}
	return out
}

// union adds b's sources to a, keeping them sorted and distinct.
func union(left, right []int) []int {
	for _, s := range right {
		if i, found := slices.BinarySearch(left, s); !found {
			left = slices.Insert(slices.Clone(left), i, s)
		}
	}
	return left
}

// rarityShare bounds what Rarity adds: MaxLines/rarityShare entries. Filling
// the whole cap nearly doubled system-log output (journalctl, dmesg) for 1-3
// more artifacts read back, since there almost every line is rare.
const rarityShare = 8

// limit keeps warnings/errors plus head and tail when still too long. With
// Rarity it adds up to MaxLines/rarityShare entries more, the ones that stand
// for the fewest lines (the highest surprisal of their template).
func (o *Optimizer) limit(es []entry) []entry {
	if len(es) <= o.cfg.MaxLines {
		return es
	}
	head, tail := o.cfg.MaxLines/4, o.cfg.MaxLines/4
	keep := make([]bool, len(es))
	kept := 0
	for i, e := range es {
		if i < head || i >= len(es)-tail || severity(e.text) != compress.None || continuation.MatchString(e.text) {
			keep[i] = true
			kept++
		}
	}
	if o.cfg.Rarity {
		for _, i := range o.rarest(es, keep, min(o.cfg.MaxLines-kept, o.cfg.MaxLines/rarityShare)) {
			keep[i] = true
		}
	}
	var out []entry
	dropped := 0
	for i, e := range es {
		if !keep[i] {
			dropped++
			continue
		}
		if dropped > 0 {
			out = append(out, entry{text: compress.Omitted(dropped, "")})
			dropped = 0
		}
		out = append(out, e)
	}
	return out
}

// rarest returns the indexes of up to room entries not kept yet, the ones
// standing for the fewest lines first, earlier first among equals. Markers
// stand for nothing and are never picked.
func (o *Optimizer) rarest(es []entry, keep []bool, room int) []int {
	var idx []int
	for i, e := range es {
		if !keep[i] && e.n > 0 {
			idx = append(idx, i)
		}
	}
	slices.SortStableFunc(idx, func(left, right int) int { return cmp.Compare(es[left].n, es[right].n) })
	return idx[:max(0, min(room, len(idx)))]
}
