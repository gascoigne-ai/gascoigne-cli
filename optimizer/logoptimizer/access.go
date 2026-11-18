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
	status compress.Tally
}

// access summarizes access logs: totals per status class, top routes with
// their statuses, top clients, and every server error line.
func (o *Optimizer) access(lines []string) []string {
	log := parseAccess(lines)
	out := []string{fmt.Sprintf("%d requests %s – %s: %s", log.total, log.first, log.last, log.classes.Counts(false))}
	out = append(out, "routes:")
	out = append(out, o.routeLines(log.routes)...)
	out = append(out, "clients: "+log.clients.Top(5))
	if len(log.errs) > 0 {
		out = append(out, "server errors:")
		out = append(out, compress.DedupeRuns(compress.HeadTail(log.errs, 15, 5))...)
	}
	if len(log.other) > 0 {
		out = append(out, "other lines:")
		out = append(out, o.log(log.other)...)
	}
	return out
}

// accessLog is what access reads out of the lines of an access log.
type accessLog struct {
	routes      map[string]*hits
	classes     compress.Tally
	clients     compress.Tally
	errs, other []string
	first, last string
	total       int
}

// parseAccess reads every line that matches accessRe into the totals; the
// others are kept as other, blank lines dropped.
func parseAccess(lines []string) *accessLog {
	log := &accessLog{routes: map[string]*hits{}, classes: compress.Tally{}, clients: compress.Tally{}}
	for _, l := range lines {
		m := accessRe.FindStringSubmatch(l)
		if m == nil {
			if strings.TrimSpace(l) != "" {
				log.other = append(log.other, l)
			}
			continue
		}
		log.add(l, m)
	}
	return log
}

// add counts one request line l whose accessRe match is m.
func (log *accessLog) add(line string, match []string) {
	log.total++
	if log.first == "" {
		log.first = match[2]
	}
	log.last = match[2]
	ip, method, status := match[1], match[3], match[5]
	log.classes[status[:1]+"xx"]++
	log.clients[ip]++
	key := method + " " + route(match[4])
	if log.routes[key] == nil {
		log.routes[key] = &hits{status: compress.Tally{}}
	}
	log.routes[key].n++
	log.routes[key].status[status]++
	if status[0] == '5' {
		log.errs = append(log.errs, line)
	}
}

// routeLines lists the busiest routes with their statuses, up to the line budget.
func (o *Optimizer) routeLines(routes map[string]*hits) []string {
	var out []string
	keys := slices.SortedFunc(maps.Keys(routes), func(left, right string) int {
		return cmp.Or(cmp.Compare(routes[right].n, routes[left].n), cmp.Compare(left, right))
	})
	for _, k := range keys[:min(len(keys), o.cfg.MaxLines/10)] {
		out = append(out, fmt.Sprintf("  %s ×%d (%s)", k, routes[k].n, routes[k].status.Counts(true)))
	}
	if n := len(keys) - o.cfg.MaxLines/10; n > 0 {
		out = append(out, "  "+compress.Omitted(n, "routes"))
	}
	return out
}
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
