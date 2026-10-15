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
	keys := slices.SortedFunc(maps.Keys(routes), func(a, b string) int {
		return cmp.Or(cmp.Compare(routes[b].n, routes[a].n), cmp.Compare(a, b))
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
func route(p string) string {
	p, _, _ = strings.Cut(p, "?")
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if idSegment.MatchString(s) {
			segs[i] = ":id"
		}
	}
	return strings.Join(segs, "/")
}

// counts renders "2xx 120, 5xx 3", by key or by count.
func counts(m map[string]int, byCount bool) string {
	keys := slices.Sorted(maps.Keys(m))
	if byCount {
		slices.SortStableFunc(keys, func(a, b string) int { return cmp.Compare(m[b], m[a]) })
	}
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, m[k])
	}
	return strings.Join(parts, ", ")
}

// top renders the n most frequent keys.
func top(m map[string]int, n int) string {
	keys := slices.SortedFunc(maps.Keys(m), func(a, b string) int { return cmp.Or(cmp.Compare(m[b], m[a]), cmp.Compare(a, b)) })
	var parts []string
	for _, k := range keys[:min(n, len(keys))] {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, m[k]))
	}
	if len(keys) > n {
		parts = append(parts, fmt.Sprintf("… %d more", len(keys)-n))
	}
	return strings.Join(parts, ", ")
}
