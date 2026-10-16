package authtest

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAuthorizeRedirectsOnlyToLoopback(t *testing.T) {
	base, _ := Server(t)
	base = strings.TrimSuffix(base, "/mcp")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	tests := []struct {
		name, redirect string
		status         int
		location       string
	}{
		{"loopback ip", "http://127.0.0.1:8123/callback", http.StatusFound, "http://127.0.0.1:8123/callback?code=c1&state=s1"},
		{"localhost", "http://localhost:8123/callback", http.StatusFound, "http://localhost:8123/callback?code=c1&state=s1"},
		{"other host", "http://evil.example/callback", http.StatusBadRequest, ""},
		{"https loopback", "https://127.0.0.1/callback", http.StatusBadRequest, ""},
		{"not a url", "%zz", http.StatusBadRequest, ""},
		{"missing", "", http.StatusBadRequest, ""},
		{"other path", "http://127.0.0.1:8123/steal", http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.Get(base + "/authorize?state=s1&redirect_uri=" + url.QueryEscape(tt.redirect))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.status || resp.Header.Get("Location") != tt.location {
				t.Errorf("status %d location %q, want %d %q", resp.StatusCode, resp.Header.Get("Location"), tt.status, tt.location)
			}
		})
	}
}
