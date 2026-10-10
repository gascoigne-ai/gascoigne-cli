// Package rows samples long lists of records (decoded JSON objects, table
// rows) down to the ones worth reading: errors, anomalies, change points,
// query matches and both ends. Gaps become "… N items omitted" markers.
package rows

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
)

// Signal scores; errors are not scored, they are always kept.
const (
	scoreChange    = 3
	scoreRelevance = 5
)

// Options tune Sample.
type Options struct {
	MaxItems   int      // target items kept; errors may exceed it
	HeadShare  float64  // share of MaxItems taken from the start
	TailShare  float64  // and from the end
	AnomalyStd float64  // numeric outliers beyond this many standard deviations
	RelevanceK int      // best query matches kept
	Query      []string // query terms (see Terms)
}

// Sample keeps about MaxItems items of a long list, in original order with
// omission markers. Errors always stay (even over budget); then anomalies,
// change points and query matches by score; then head and tail shares; then
// an even sample of the rest. Identical items collapse into one.
// Items are decoded JSON values; objects (map[string]any with json.Number
// numbers) feed the signals.
func Sample(items []any, o Options) []any {
	uniq, firstOf, dups := dedupe(items)
	objs := objects(items, uniq)

	keep := map[int]bool{}
	for _, i := range slices.Concat(errorItems(objs), rareKeys(objs), rareValues(objs)) {
		keep[i] = true
	}
	score := map[int]float64{}
	for i, z := range anomalies(objs, o.AnomalyStd) {
		score[i] = max(score[i], z)
	}
	for _, i := range changePoints(objs, o.MaxItems) {
		score[i] += scoreChange
	}
	for i, s := range relevance(items, uniq, o.Query, o.RelevanceK) {
		score[i] += scoreRelevance + s
	}

	budget := o.MaxItems
	head := int(math.Ceil(o.HeadShare * float64(budget)))
	tail := int(math.Ceil(o.TailShare * float64(budget)))
	slots := budget - len(keep) - head - tail // errors count against the budget but never drop
	for _, i := range sortedByScore(score) {
		if slots <= 0 {
			break
		}
		if !keep[i] {
			keep[i] = true
			slots--
		}
	}
	for _, i := range uniq[:min(head, len(uniq))] {
		keep[i] = true
	}
	for _, i := range uniq[max(0, len(uniq)-tail):] {
		keep[i] = true
	}
	fill(keep, uniq, budget)
	return render(items, keep, firstOf, dups)
}

// dedupe returns first-occurrence indices, each item's first occurrence and
// how many later items repeat each first occurrence.
func dedupe(items []any) (uniq, firstOf []int, dups map[int]int) {
	byHash := map[uint64][]int{}
	firstOf = make([]int, len(items))
	dups = map[int]int{}
	for i, it := range items {
		h := hash(it)
		j := slices.IndexFunc(byHash[h], func(j int) bool { return reflect.DeepEqual(items[j], it) })
		if j >= 0 {
			f := byHash[h][j]
			firstOf[i] = f
			dups[f]++
			continue
		}
		byHash[h] = append(byHash[h], i)
		firstOf[i] = i
		uniq = append(uniq, i)
	}
	return uniq, firstOf, dups
}

// object is an array item that is a JSON object, with its index.
type object struct {
	i int
	m map[string]any
}

func objects(items []any, uniq []int) []object {
	var objs []object
	for _, i := range uniq {
		if m, ok := items[i].(map[string]any); ok {
			objs = append(objs, object{i, m})
		}
	}
	return objs
}

// fill adds evenly spaced items until budget is reached.
func fill(keep map[int]bool, uniq []int, budget int) {
	var rest []int
	for _, i := range uniq {
		if !keep[i] {
			rest = append(rest, i)
		}
	}
	slots := budget - len(keep)
	if slots <= 0 || len(rest) == 0 {
		return
	}
	step := float64(len(rest)) / float64(slots+1)
	for k := 1; k <= slots && int(float64(k)*step) < len(rest); k++ {
		keep[rest[int(float64(k)*step)]] = true
	}
}

// render emits kept items in order; dropped runs become one marker, repeats
// of a kept item a marker right after it.
func render(items []any, keep map[int]bool, firstOf []int, dups map[int]int) []any {
	var out []any
	gap := 0
	for i, it := range items {
		if f := firstOf[i]; f != i && keep[f] {
			continue // counted in the marker after its kept original
		}
		if !keep[i] {
			gap++
			continue
		}
		if gap > 0 {
			out = append(out, compress.Omitted(gap, "items"))
			gap = 0
		}
		out = append(out, it)
		if n := dups[i]; n > 0 {
			out = append(out, fmt.Sprintf("… %d identical items omitted", n))
		}
	}
	if gap > 0 {
		out = append(out, compress.Omitted(gap, "items"))
	}
	return out
}

func sortedByScore(score map[int]float64) []int {
	idx := make([]int, 0, len(score))
	for i := range score {
		idx = append(idx, i)
	}
	slices.SortFunc(idx, func(a, b int) int {
		return cmp.Or(cmp.Compare(score[b], score[a]), cmp.Compare(a, b))
	})
	return idx
}

var (
	errorKeys  = map[string]bool{"error": true, "errors": true, "err": true, "exception": true, "failure": true, "fault": true}
	statusKeys = map[string]bool{"status": true, "state": true, "level": true, "severity": true, "result": true, "outcome": true, "conclusion": true}
	codeKeys   = map[string]bool{"status": true, "status_code": true, "statuscode": true, "code": true, "http_status": true}
	badStatus  = regexp.MustCompile(`(?i)error|fail|fatal|crit|panic|declin|denied|reject|timeout|abort`)
)

// errorItems finds error-like items: an error field, a bad status word or an
// HTTP 4xx/5xx code.
func errorItems(objs []object) []int {
	bad := map[string]bool{} // statuses repeat: match each distinct value once
	isBadStatus := func(v any) bool {
		s, ok := v.(string)
		if !ok || len(s) >= 40 {
			return false
		}
		b, seen := bad[s]
		if !seen {
			b = badStatus.MatchString(s)
			bad[s] = b
		}
		return b
	}
	var idx []int
	for _, o := range objs {
		for k, v := range o.m {
			k = strings.ToLower(k)
			if errorKeys[k] && !empty(v) && v != false ||
				statusKeys[k] && isBadStatus(v) ||
				codeKeys[k] && isHTTPError(v) {
				idx = append(idx, o.i)
				break
			}
		}
	}
	return idx
}

func isHTTPError(v any) bool {
	n, ok := v.(json.Number)
	if !ok {
		return false
	}
	c, err := n.Int64()
	return err == nil && c >= 400 && c < 600
}

// rareKeys finds items carrying a key few others have (≤5%): structural
// outliers such as an "error" or "retry" field only failing items carry.
func rareKeys(objs []object) []int {
	if len(objs) < 10 {
		return nil
	}
	count := map[string]int{}
	for _, o := range objs {
		for k := range o.m {
			count[k]++
		}
	}
	limit := max(1, len(objs)/20)
	var idx []int
	for _, o := range objs {
		for k := range o.m {
			if count[k] <= limit {
				idx = append(idx, o.i)
				break
			}
		}
	}
	return idx
}

// rareValues finds items with a value seen at most 1% of the time in a
// low-cardinality string field (an enum), such as the few "error" levels.
func rareValues(objs []object) []int {
	if len(objs) < 10 {
		return nil
	}
	counts := map[string]map[string]int{}
	for _, o := range objs {
		for k, v := range o.m {
			if s, ok := v.(string); ok {
				if counts[k] == nil {
					counts[k] = map[string]int{}
				}
				counts[k][s]++
			}
		}
	}
	limit := max(1, len(objs)/100)
	var idx []int
	for _, o := range objs {
		for k, v := range o.m {
			s, ok := v.(string)
			if ok && len(counts[k]) > 1 && len(counts[k]) <= 10 && counts[k][s] <= limit {
				idx = append(idx, o.i)
				break
			}
		}
	}
	return idx
}

// numeric collects a numeric field per item, for fields most items have.
func numeric(objs []object) map[string][]point {
	fields := map[string][]point{}
	for _, o := range objs {
		for k, v := range o.m {
			if n, ok := v.(json.Number); ok {
				if f, err := n.Float64(); err == nil {
					fields[k] = append(fields[k], point{o.i, f})
				}
			}
		}
	}
	for k, pts := range fields {
		if len(pts) < 10 || 5*len(pts) < 4*len(objs) {
			delete(fields, k)
		}
	}
	return fields
}

type point struct {
	i int
	v float64
}

// anomalies scores items whose numeric fields lie more than k standard
// deviations from the field mean; the score is the largest z.
func anomalies(objs []object, k float64) map[int]float64 {
	out := map[int]float64{}
	for _, pts := range numeric(objs) {
		mean, std := stats(pts, func(p point) float64 { return p.v })
		if std == 0 {
			continue
		}
		for _, p := range pts {
			if z := math.Abs(p.v-mean) / std; z > k {
				out[p.i] = max(out[p.i], z)
			}
		}
	}
	return out
}

// changePoints finds where a series shifts: a numeric step far above the
// usual step size, or a low-cardinality field changing value. Noisy fields
// with more transitions than limit say nothing and are skipped.
func changePoints(objs []object, limit int) []int {
	var idx []int
	for _, pts := range numeric(objs) {
		var steps []point
		for j := 1; j < len(pts); j++ {
			steps = append(steps, point{pts[j].i, math.Abs(pts[j].v - pts[j-1].v)})
		}
		mean, std := stats(steps, func(p point) float64 { return p.v })
		var found []int
		for _, s := range steps {
			if std > 0 && s.v > mean+3*std {
				found = append(found, s.i)
			}
		}
		if len(found) <= limit {
			idx = append(idx, found...)
		}
	}
	values := map[string][]string{}
	for _, o := range objs {
		for k, v := range o.m {
			if s, ok := v.(string); ok {
				values[k] = append(values[k], s)
			}
		}
	}
	for _, vals := range values {
		if len(vals) != len(objs) || distinct(vals) > 10 || distinct(vals) < 2 {
			continue
		}
		var found []int
		for j := 1; j < len(vals); j++ {
			if vals[j] != vals[j-1] {
				found = append(found, objs[j].i)
			}
		}
		if len(found) <= limit/2 {
			idx = append(idx, found...)
		}
	}
	return idx
}

func distinct(vals []string) int {
	seen := map[string]bool{}
	for _, v := range vals {
		seen[v] = true
	}
	return len(seen)
}

func stats[T any](xs []T, f func(T) float64) (mean, std float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += f(x)
	}
	mean /= float64(len(xs))
	for _, x := range xs {
		d := f(x) - mean
		std += d * d
	}
	return mean, math.Sqrt(std / float64(len(xs)))
}

func empty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}
