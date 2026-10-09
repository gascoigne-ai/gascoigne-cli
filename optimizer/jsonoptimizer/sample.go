package jsonoptimizer

import (
	"encoding/json"

	"math"
	"regexp"
	"slices"

	"github.com/lukaszb/gascoigne/compress"
)

var (
	errorKey   = regexp.MustCompile(`(?i)^(error|errors|err|exception|failure|fault)$`)
	statusKey  = regexp.MustCompile(`(?i)^(status|state|level|severity|result|outcome|conclusion)$`)
	badStatus  = regexp.MustCompile(`(?i)error|fail|fatal|critical|crit|panic|declin|denied|reject|timeout|abort`)
	statusCode = regexp.MustCompile(`(?i)^(status|status_?code|code|http_?status)$`)
)

// sample keeps at most MaxItems items: errors first, then numeric outliers,
// then rare values, then both ends. Gaps become "… N items omitted" strings.
func (o *Optimizer) sample(items []any) []any {
	if len(items) <= o.cfg.MaxItems {
		return items
	}
	keep := map[int]bool{}
	add := func(idx []int) {
		for _, i := range idx {
			if len(keep) < o.cfg.MaxItems {
				keep[i] = true
			}
		}
	}
	add(errorItems(items))
	add(outliers(items))
	add(rare(items))
	ends := []int{}
	for i := range o.cfg.KeepEnds {
		ends = append(ends, i, len(items)-1-i)
	}
	add(ends)

	var out []any
	gap := 0
	for i, it := range items {
		if !keep[i] {
			gap++
			continue
		}
		if gap > 0 {
			out = append(out, compress.Omitted(gap, "items"))
			gap = 0
		}
		out = append(out, it)
	}
	if gap > 0 {
		out = append(out, compress.Omitted(gap, "items"))
	}
	return out
}

func errorItems(items []any) []int {
	var idx []int
	for i, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for k, v := range obj {
			if errorKey.MatchString(k) && !empty(v) && v != false {
				idx = append(idx, i)
				break
			}
			if s, ok := v.(string); ok && statusKey.MatchString(k) && badStatus.MatchString(s) {
				idx = append(idx, i)
				break
			}
			if n, ok := v.(json.Number); ok && statusCode.MatchString(k) {
				if c, err := n.Int64(); err == nil && c >= 400 && c < 600 {
					idx = append(idx, i)
					break
				}
			}
		}
	}
	return idx
}

// outliers returns items with a top-level numeric field more than 3 standard
// deviations from that field's mean.
func outliers(items []any) []int {
	if len(items) < 10 {
		return nil
	}
	fields := map[string][]float64{}
	for _, it := range items {
		if obj, ok := it.(map[string]any); ok {
			for k, v := range obj {
				if n, ok := v.(json.Number); ok {
					if f, err := n.Float64(); err == nil {
						fields[k] = append(fields[k], f)
					}
				}
			}
		}
	}
	var idx []int
	for _, k := range sortedKeys(fields) {
		vals := fields[k]
		if len(vals) < 10 {
			continue
		}
		mean, std := stats(vals)
		if std == 0 {
			continue
		}
		for i, it := range items {
			obj, _ := it.(map[string]any)
			if n, ok := obj[k].(json.Number); ok {
				if f, _ := n.Float64(); math.Abs(f-mean) > 3*std {
					idx = append(idx, i)
				}
			}
		}
	}
	return idx
}

// rare returns items holding a value seen once in a low-cardinality string
// field (an enum), such as the single "error" level among "info"s.
func rare(items []any) []int {
	if len(items) < 20 {
		return nil
	}
	counts := map[string]map[string]int{}
	for _, it := range items {
		if obj, ok := it.(map[string]any); ok {
			for k, v := range obj {
				if s, ok := v.(string); ok {
					if counts[k] == nil {
						counts[k] = map[string]int{}
					}
					counts[k][s]++
				}
			}
		}
	}
	var idx []int
	for _, k := range sortedKeys(counts) {
		if len(counts[k]) > 10 {
			continue // not an enum
		}
		for i, it := range items {
			obj, _ := it.(map[string]any)
			if s, ok := obj[k].(string); ok && counts[k][s] == 1 {
				idx = append(idx, i)
			}
		}
	}
	return idx
}

func stats(vals []float64) (mean, std float64) {
	for _, v := range vals {
		mean += v
	}
	mean /= float64(len(vals))
	for _, v := range vals {
		std += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(std / float64(len(vals)))
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
