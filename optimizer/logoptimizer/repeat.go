package logoptimizer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

// maxNamed is how many overflow variants a folded line names.
const maxNamed = 6

// repeatKeys returns the key under which each line counts as a repeat: its
// normalized form, or with the drain miner its template cluster plus its
// words at the places the cluster varies. Only the first DrainVariants
// variants of a cluster keep a key of their own; later ones share one, and
// notes names them, so folding never hides which user or device a line was.
// Warnings, errors and outliers stay on the exact key so they never merge.
func (o *Optimizer) repeatKeys(lines, norms []string, important []bool) (keys []string, notes map[string]string) {
	if o.cfg.Miner != minerDrain || o.norm != nil {
		return norms, nil
	}
	m := newDrain(drainConfig{Depth: o.cfg.DrainDepth, Similarity: o.cfg.DrainSimilarity, MaxChildren: o.cfg.DrainMaxChildren, MaxVars: o.cfg.DrainMaxVars})
	ids := make([]int, len(lines))
	for i, l := range lines {
		if important[i] {
			continue
		}
		tag := ""
		if isDebug(l) {
			tag = "debug"
		}
		ids[i] = m.add(tag, norms[i])
	}
	keys = slices.Clone(norms)
	owners := map[int][]string{} // cluster -> variants with a key of their own
	named := map[string][]string{}
	for i := range lines {
		if important[i] {
			continue
		}
		variant := strings.Join(m.words(ids[i], norms[i]), " ")
		own := owners[ids[i]]
		if !slices.Contains(own, variant) && len(own) < o.cfg.DrainVariants {
			own = append(own, variant)
			owners[ids[i]] = own
		}
		key := fmt.Sprintf("\x00drain%d %s", ids[i], variant)
		if !slices.Contains(own, variant) {
			key = fmt.Sprintf("\x00drain%d more", ids[i])
			if !slices.Contains(named[key], variant) {
				named[key] = append(named[key], variant)
			}
		}
		keys[i] = key
	}
	return keys, variantNotes(named)
}

// variantNotes names, per shared key, the variants beyond the one shown.
func variantNotes(named map[string][]string) map[string]string {
	notes := map[string]string{}
	for k, vs := range named {
		if len(vs) < 2 {
			continue // the line shown is the only variant
		}
		others := vs[1:]
		shown := others[:min(len(others), maxNamed)]
		note := "also " + strings.Join(shown, ", ")
		if extra := len(others) - len(shown); extra > 0 {
			note += fmt.Sprintf(", +%d", extra)
		}
		notes[k] = note
	}
	return notes
}

// repeatNotes are the remarks after ×N: the variants a folded drain line
// names and, with Bursts, where the repeats cluster.
func (o *Optimizer) repeatNotes(variants string, at []int, total int) []string {
	var notes []string
	if variants != "" {
		notes = append(notes, variants)
	}
	if first, last, ok := burst(at, len(at), total); ok {
		notes = append(notes, fmt.Sprintf("mostly lines %d–%d", first+1, last+1))
	}
	return notes
}

// gap marks dropped lines. When each repeats a line shown above (and counted
// there with ×N), saying so tells the reader nothing new is hidden; a mixed
// run stays "omitted", the counts above already tell its repeats.
func gap(dropped, repeats int) string {
	switch {
	case repeats < dropped:
		return compress.Omitted(dropped, "")
	case dropped == 1:
		return "… 1 repeat of a line above"
	}
	return fmt.Sprintf("… %d repeats of lines above", dropped)
}
