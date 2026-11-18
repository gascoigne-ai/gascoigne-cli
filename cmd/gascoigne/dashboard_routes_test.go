package main

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// dashboardRoutes is every route the dashboard mux answers: path to methods.
// It pins the route list across refactors of how the mux is built.
var dashboardRoutes = map[string][]string{
	"/":                    {"GET"},
	"/mcp":                 {"GET"},
	"/costs":               {"GET"},
	"/context":             {"GET"},
	"/setup":               {"GET"},
	"/assets/pages.css":    {"GET"},
	"/assets/pages.js":     {"GET"},
	"/api/health":          {"GET"},
	"/api/experiments":     {"GET"},
	"/api/shutdown":        {"POST"},
	"/api/reset":           {"POST"},
	"/api/originals":       {"GET"},
	"/api/flush":           {"POST"},
	"/api/setup":           {"GET", "POST"},
	"/api/setup/link":      {"POST"},
	"/api/costs":           {"GET"},
	"/api/pricing":         {"GET"},
	"/api/pricing/refresh": {"POST"},
	"/api/context":         {"GET"},
	"/api/mcp":             {"GET"},
	"/api/mcp/lazy":        {"POST"},
	"/api/mcp/learn":       {"POST"},
	"/api/mcp/rules":       {"POST"},
	"/api/mcp/optimize":    {"POST"},
	"/api/mcp/login":       {"GET", "POST"},
	"/api/overview":        {"GET"},
	"/api/usage":           {"GET"},
	"/api/impact":          {"GET"},
	"/api/sessions":        {"GET"},
	"/api/events":          {"GET"},
	"/api/stream":          {"GET"},
	"/api/raw/some-id":     {"GET"},
	"/api/out/some-id":     {"GET"},
	"/api/failures":        {"GET"},
	"/api/forget":          {"POST"},
}

// A method no route uses makes the mux answer 405 with an Allow header for a
// known path and 404 for an unknown one, without running any handler.
func TestDashboardRouteList(t *testing.T) {
	sandbox(t)
	_, handler := testDashboard(t)
	for path, methods := range dashboardRoutes {
		t.Run(path, func(t *testing.T) {
			recorder := get(handler, "PATCH", path)
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("%d, want 405", recorder.Code)
			}
			want := slices.Clone(methods)
			if slices.Contains(want, "GET") {
				want = append(want, "HEAD")
			}
			got := strings.Split(recorder.Header().Get("Allow"), ", ")
			slices.Sort(want)
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Fatalf("allow %v, want %v", got, want)
			}
		})
	}
	for _, path := range []string{"/nowhere", "/api/nowhere", "/assets/other.js"} {
		if recorder := get(handler, "PATCH", path); recorder.Code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", path, recorder.Code)
		}
	}
}

func TestDashboardStaticPages(t *testing.T) {
	sandbox(t)
	_, handler := testDashboard(t)
	tests := []struct {
		path, contentType, cache string
		body                     []byte
	}{
		{"/", "text/html; charset=utf-8", "", dashboardHTML},
		{"/mcp", "text/html; charset=utf-8", "", mcpHTML},
		{"/costs", "text/html; charset=utf-8", "", costsHTML},
		{"/context", "text/html; charset=utf-8", "", contextHTML},
		{"/setup", "text/html; charset=utf-8", "", setupHTML},
		{"/assets/pages.css", "text/css; charset=utf-8", "no-cache", pagesCSS},
		{"/assets/pages.js", "text/javascript; charset=utf-8", "no-cache", pagesJS},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			recorder := get(handler, "GET", tt.path)
			if recorder.Code != http.StatusOK {
				t.Fatalf("%d", recorder.Code)
			}
			if got := recorder.Header().Get("Content-Type"); got != tt.contentType {
				t.Errorf("content type %q, want %q", got, tt.contentType)
			}
			if got := recorder.Header().Get("Cache-Control"); got != tt.cache {
				t.Errorf("cache control %q, want %q", got, tt.cache)
			}
			if recorder.Body.String() != string(tt.body) {
				t.Error("body differs from the embedded file")
			}
		})
	}
}
