package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcpauth/authtest"
)

// The proxy asks the server to log in; the server opens the browser.
func TestLoginRunsInTheServer(t *testing.T) {
	sandbox(t)
	url, _ := authtest.Server(t)
	var mu sync.Mutex
	var opened []string
	flows := newLoginFlows(authStore(), func(authURL string) bool {
		mu.Lock()
		opened = append(opened, authURL)
		mu.Unlock()
		go func() { // the "browser"
			if resp, err := http.Get(authURL); err == nil {
				resp.Body.Close()
			}
		}()
		return true
	})
	mux := http.NewServeMux()
	flows.handlers(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var shown string
	err := loginViaServer(t.Context(), strings.TrimPrefix(srv.URL, "http://"), url, func(a string) { shown = a })
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(opened) != 1 || shown != opened[0] || !strings.Contains(shown, "/authorize?") {
		t.Fatalf("opened %q, shown %q", opened, shown)
	}
	if _, ok := authStore().Get(url); !ok {
		t.Fatal("login not saved")
	}
}

func TestLoginWithoutServerIsUnavailable(t *testing.T) {
	sandbox(t)
	err := loginViaServer(t.Context(), "127.0.0.1:1", "https://mcp.example.com/mcp", func(string) {})
	if err == nil || !strings.Contains(err.Error(), errUnavailable.Error()) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoginRejectsOtherSchemes(t *testing.T) {
	mux := http.NewServeMux()
	newLoginFlows(authStore(), func(string) bool { return false }).handlers(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	resp, err := http.Post(srv.URL+"/api/mcp/login", "application/json", strings.NewReader(`{"url":"file:///etc/passwd"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
