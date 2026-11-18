package detect

import (
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// ariaNode is one node of an accessibility snapshot: - link "Home" [ref=e12]
var ariaNode = lazyre.New(`(?m)^\s*- [a-z]+( "[^"\n]*")?( \[[\w-]+(=[^\]\n]*)?\])* \[ref=e\d+\]`)

// Web content.
const (
	HTML = "web/html"
	HTTP = "web/http" // responses with headers: curl -i/-v/-I, httpie -v
	ARIA = "web/aria" // accessibility snapshots of browser automation tools: role trees with [ref=eN]
	// SEARCH is what an agent's web search tool returns as text: a header with the query, a
	// Links: line holding a JSON list of {title,url}, then the model's summary.
	SEARCH = "web/search"
)

func init() {
	register(
		Spec{Type: ARIA, Priority: 16, Refines: []string{"data/yaml", Markdown, Text},
			Sniff: func(text string, _ []string) bool { return len(ariaNode.FindAllStringIndex(text, 8)) >= 8 }},
		Spec{Type: SEARCH, Priority: 15, Refines: []string{Markdown, Text},
			Sniff: func(text string, _ []string) bool {
				return strings.HasPrefix(text, `Web search results for query: "`) && strings.Contains(text, "\nLinks: [")
			}},
		Spec{Type: HTTP, Priority: 17,
			CmdFunc: func(args []string) bool {
				switch args[0] {
				case "curl":
					return slices.ContainsFunc(args[1:], func(arg string) bool {
						return arg == "--include" || arg == "--head" || arg == "--verbose" || strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsAny(arg, "iIv")
					})
				case "http", "https", "xh":
					return slices.Contains(args[1:], "-v") || slices.Contains(args[1:], "--verbose") || slices.Contains(args[1:], "--headers")
				}
				return false
			},
			Sniff: anyLine(`^(< )?HTTP/[\d.]+ \d{3}( |$)`)},
		Spec{Type: HTML, Files: []string{"*.html", "*.htm"}, MIME: []string{"text/html", "application/xhtml+xml"}, Priority: 30,
			Sniff: func(text string, _ []string) bool {
				head := strings.ToLower(text[:min(len(text), 2048)])
				return containsAny(head, "<!doctype html", "<html", "<body", "<head")
			}},
	)
}

func containsAny(text string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(text, sub) {
			return true
		}
	}
	return false
}

func containsAll(text string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(text, sub) {
			return false
		}
	}
	return true
}
