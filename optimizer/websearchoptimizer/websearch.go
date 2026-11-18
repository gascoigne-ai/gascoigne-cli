// Package websearchoptimizer is the web family engine for an agent's web
// search results as text: `Web search results for query: "…"`, a `Links:`
// line with a JSON list of {title,url}, the model's summary, a reminder to
// cite. The summary and reminder stay byte-exact (the reminder is an
// instruction); each Links line becomes one `- title: url` line per link,
// without a title that only repeats the host and without repeated URLs. A
// hit has no snippet in this shape, so there is none to trim. Over
// `max_links`, the links matching the query best stay (ties go to the
// engine's earlier, better-ranked ones) in the engine's order.
package websearchoptimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	query := sample.Terms(data.Attrs[payload.AttrQuery])
	return data.Rewrite(func(block payload.Block) (string, error) {
		return o.shrink(block.Text, query), nil
	}, detect.SEARCH)
}

type link struct{ Title, URL string }

const linksPrefix = "Links: ["

// shrink rewrites each Links line of text; other lines pass through.
func (o *Optimizer) shrink(text string, query []string) string {
	lines := strings.SplitAfter(text, "\n")
	for i, l := range lines {
		raw := strings.TrimRight(l, "\n")
		if !strings.HasPrefix(raw, linksPrefix) {
			continue
		}
		var links []link
		if json.Unmarshal([]byte(raw[len("Links: "):]), &links) != nil || slices.ContainsFunc(links, func(item link) bool { return item.URL == "" }) {
			continue
		}
		lines[i] = o.render(links, query) + l[len(raw):]
	}
	return strings.Join(lines, "")
}

// render lists the links, one per line, under a `Links:` line (no trailing newline).
func (o *Optimizer) render(links []link, query []string) string {
	links = dedupe(links)
	kept := o.pick(links, query)
	out := []string{"Links:"}
	for i, k := range links {
		if kept[i] {
			out = append(out, "- "+describe(k))
		}
	}
	if n := len(links) - len(out) + 1; n > 0 {
		out = append(out, fmt.Sprintf("- … %d more links dropped", n))
	}
	return strings.Join(out, "\n")
}

// pick marks the links to keep: all within max_links, else the best by BM25
// of title and URL against the query, earlier ones first on a tie.
func (o *Optimizer) pick(links []link, query []string) []bool {
	kept := make([]bool, len(links))
	if len(links) <= o.cfg.MaxLinks {
		for i := range kept {
			kept[i] = true
		}
		return kept
	}
	docs := make([][]string, len(links))
	for i, k := range links {
		docs[i] = sample.Terms(k.Title + " " + k.URL)
	}
	scores := sample.BM25(docs, query)
	order := make([]int, len(links))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(left, right int) int {
		switch {
		case scores[left] > scores[right]:
			return -1
		case scores[left] < scores[right]:
			return 1
		}
		return 0
	})
	for _, i := range order[:o.cfg.MaxLinks] {
		kept[i] = true
	}
	return kept
}

// dedupe drops links whose URL was listed before.
func dedupe(links []link) []link {
	seen := map[string]bool{}
	return slices.DeleteFunc(slices.Clone(links), func(item link) bool {
		dup := seen[item.URL]
		seen[item.URL] = true
		return dup
	})
}

// describe is `title: url`, or the URL alone when the title is empty or only
// the host (search engines title bare pages that way).
func describe(item link) string {
	if item.Title == "" || hostOnly(item) {
		return item.URL
	}
	return item.Title + ": " + item.URL
}

func hostOnly(item link) bool {
	u, err := url.Parse(item.URL)
	if err != nil {
		return false
	}
	return strings.EqualFold(strings.TrimPrefix(u.Hostname(), "www."), strings.TrimPrefix(strings.TrimSpace(item.Title), "www."))
}
