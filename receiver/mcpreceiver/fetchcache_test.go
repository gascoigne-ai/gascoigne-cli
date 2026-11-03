package mcpreceiver

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A repeat fetch of the same URL in a session used to refetch and re-optimize
// in full; it now points at the earlier result or sends only the changed lines.
func TestFetchSessionCache(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d of the page", i)
	}
	body := strings.Join(lines, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer srv.Close()
	cs, _, _ := connect(t)
	fetch := func(args map[string]any) string {
		args["url"] = srv.URL
		text, isErr := call(t, cs, "fetch", args)
		if isErr {
			t.Fatalf("fetch error: %s", text)
		}
		return text
	}

	if text := fetch(map[string]any{}); !strings.Contains(text, "[raw:") {
		t.Fatalf("first fetch = %q", text)
	}
	if text := fetch(map[string]any{}); !strings.Contains(text, "unchanged since") || !strings.Contains(text, "id=id1") || len(text) > 200 {
		t.Fatalf("repeat fetch = %q", text)
	}
	if text := fetch(map[string]any{"raw": true}); strings.Contains(text, "unchanged") || !strings.HasSuffix(text, "of the page") {
		t.Fatalf("raw fetch = %q", text)
	}
	if text := fetch(map[string]any{"query": "other"}); !strings.Contains(text, "[raw:") {
		t.Fatalf("other query = %q", text)
	}

	lines[7] = "line 7 changed"
	body = strings.Join(lines, "\n")
	text := fetch(map[string]any{})
	if !strings.Contains(text, "changed since") || !strings.Contains(text, "- LINE 7 OF THE PAGE\n+ LINE 7 CHANGED") || strings.Contains(text, "LINE 8 OF") {
		t.Fatalf("changed fetch = %q", text)
	}

	body = "something else entirely"
	if text := fetch(map[string]any{}); !strings.Contains(text, "[raw:") || strings.Contains(text, "changed since") {
		t.Fatalf("rewritten fetch = %q", text)
	}
}

func TestLineDiff(t *testing.T) {
	tests := []struct{ name, a, b, want string }{
		{"replace", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj", "a\nb\nX\nd\ne\nf\ng\nh\ni\nj", "- c\n+ X"},
		{"insert", "a\nb\nc\nd\ne\nf\ng\nh", "a\nb\nc\nnew\nd\ne\nf\ng\nh", "+ new"},
		{"delete", "a\nb\nc\nd\ne\nf\ng\nh", "a\nb\nd\ne\nf\ng\nh", "- c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := lineDiff(tt.a, tt.b)
			if !ok || got != tt.want {
				t.Fatalf("lineDiff = %q, %v; want %q", got, ok, tt.want)
			}
		})
	}
	if _, ok := lineDiff("a\nb", "x\ny"); ok {
		t.Fatal("a diff as big as the text was accepted")
	}
}
