package detect

import (
	"slices"
	"strings"
)

// Web content.
const (
	HTML = "web/html"
	HTTP = "web/http" // responses with headers: curl -i/-v/-I, httpie -v
)

func init() {
	register(
		Spec{Type: HTTP, Priority: 17,
			CmdFunc: func(args []string) bool {
				switch args[0] {
				case "curl":
					return slices.ContainsFunc(args[1:], func(a string) bool {
						return a == "--include" || a == "--head" || a == "--verbose" || strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.ContainsAny(a, "iIv")
					})
				case "http", "https", "xh":
					return slices.Contains(args[1:], "-v") || slices.Contains(args[1:], "--verbose") || slices.Contains(args[1:], "--headers")
				}
				return false
			},
			Sniff: anyLine(`^(< )?HTTP/[\d.]+ \d{3}( |$)`)},
		Spec{Type: HTML, Files: []string{"*.html", "*.htm"}, MIME: []string{"text/html", "application/xhtml+xml"}, Priority: 30,
			Sniff: func(t string, _ []string) bool {
				head := strings.ToLower(t[:min(len(t), 2048)])
				return containsAny(head, "<!doctype html", "<html", "<body", "<head")
			}},
	)
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
