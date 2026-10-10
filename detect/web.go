package detect

import "strings"

// Web content.
const HTML = "web/html"

func init() {
	register(
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
