package logoptimizer

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	accessRe = lazyre.New(`^(\S+) \S+ \S+ \[([^\]]+)\] "([A-Z]+) (\S+)[^"]*" (\d{3}) (\d+|-)`)
	// Path segments that are ids: numbers, uuids, hashes, long tokens.
	idSegment = lazyre.New(`^(\d+|[0-9a-f]{8}-[0-9a-f-]{27}|[0-9a-fA-F]{12,}|[\w-]{24,})$`)
)

type hits struct {
	n      int
	status map[string]int
}

// access summarizes access logs: totals per status class, top routes with
// their statuses, top clients, and every server error line.
func (o *Optimizer) access(lines []string) []string {
	routes := map[string]*hits{}
	classes := map[string]int{}
	clients := map[string]int{}
	var errs, other []string
	first, last, total := "", "", 0
	for _, l := range lines {
		m := accessRe.FindStringSubmatch(l)
		if m == nil {
			if strings.TrimSpace(l) != "" {
				other = append(other, l)
			}
			continue
		}
		total++
		if first == "" {
			first = m[2]
		}
		last = m[2]
		ip, method, status := m[1], m[3], m[5]
		classes[status[:1]+"xx"]++
		clients[ip]++
		key := method + " " + route(m[4])
		if routes[key] == nil {
			routes[key] = &hits{status: map[string]int{}}
		}
		routes[key].n++
		routes[key].status[status]++
		if status[0] == '5' {
			errs = append(errs, l)
		}
	}
	out := []string{fmt.Sprintf("%d requests %s – %s: %s", total, first, last, counts(classes, false))}
	out = append(out, "routes:")
	keys := slices.SortedFunc(maps.Keys(routes), func(left, right string) int {
		return cmp.Or(cmp.Compare(routes[right].n, routes[left].n), cmp.Compare(left, right))
	})
	for _, k := range keys[:min(len(keys), o.cfg.MaxLines/10)] {
		out = append(out, fmt.Sprintf("  %s ×%d (%s)", k, routes[k].n, counts(routes[k].status, true)))
	}
	if n := len(keys) - o.cfg.MaxLines/10; n > 0 {
		out = append(out, "  "+compress.Omitted(n, "routes"))
	}
	out = append(out, "clients: "+top(clients, 5))
	if len(errs) > 0 {
		out = append(out, "server errors:")
		out = append(out, compress.DedupeRuns(compress.HeadTail(errs, 15, 5))...)
	}
	if len(other) > 0 {
		out = append(out, "other lines:")
		out = append(out, o.log(other)...)
	}
	return out
}

// route replaces id segments of a path and drops the query string.
func route(path string) string {
	path, _, _ = strings.Cut(path, "?")
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if idSegment.MatchString(s) {
			segs[i] = ":id"
		}
	}
	return strings.Join(segs, "/")
}

// counts renders "2xx 120, 5xx 3", by key or by count.
func counts(tally map[string]int, byCount bool) string {
	keys := slices.Sorted(maps.Keys(tally))
	if byCount {
		slices.SortStableFunc(keys, func(left, right string) int { return cmp.Compare(tally[right], tally[left]) })
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, tally[k])
	}
	return strings.Join(parts, ", ")
}

// top renders the n most frequent keys.
func top(tally map[string]int, limit int) string {
	keys := slices.SortedFunc(maps.Keys(tally), func(left, right string) int {
		return cmp.Or(cmp.Compare(tally[right], tally[left]), cmp.Compare(left, right))
	})
	var parts []string
	for _, k := range keys[:min(limit, len(keys))] {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, tally[k]))
	}
	if len(keys) > limit {
		parts = append(parts, fmt.Sprintf("… %d more", len(keys)-limit))
	}
	return strings.Join(parts, ", ")
}
