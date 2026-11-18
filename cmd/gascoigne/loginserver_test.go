package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
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
	err := loginViaServer(t.Context(), strings.TrimPrefix(srv.URL, "http://"), url, func(text string) { shown = text })
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

func TestLoginPhaseTransitions(t *testing.T) {
	phases := []loginPhase{loginRunning, loginDone, loginFailed}
	allowed := map[[2]loginPhase]bool{
		{loginRunning, loginDone}:   true,
		{loginRunning, loginFailed}: true,
	}
	for _, from := range phases {
		for _, to := range phases {
			if got, want := from.canMoveTo(to), allowed[[2]loginPhase{from, to}]; got != want {
				t.Errorf("%s -> %s: allowed %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestLoginStateJSONRoundTrip(t *testing.T) {
	for _, tt := range []struct {
		phase loginPhase
		wire  string
	}{
		{loginRunning, `{"state":"running"}`},
		{loginDone, `{"state":"done"}`},
		{loginFailed, `{"state":"failed"}`},
	} {
		got, err := json.Marshal(loginState{State: tt.phase})
		if err != nil || string(got) != tt.wire {
			t.Errorf("marshal %s = %s, %v; want %s", tt.phase, got, err, tt.wire)
		}
		var back loginState
		if err := json.Unmarshal([]byte(tt.wire), &back); err != nil || back.State != tt.phase {
			t.Errorf("unmarshal %s = %q, %v", tt.wire, back.State, err)
		}
	}
	var bad loginState
	if err := json.Unmarshal([]byte(`{"state":"bogus"}`), &bad); err == nil {
		t.Error("unknown phase accepted")
	}
}

func TestEventJSONOutcomeWire(t *testing.T) {
	got, err := json.Marshal(eventsJSON([]ledger.Entry{{Outcome: component.Failed}}))
	if err != nil || !strings.Contains(string(got), `"outcome":"error"`) {
		t.Errorf("got %s, %v", got, err)
	}
	var back eventJSON
	if err := json.Unmarshal([]byte(`{"outcome":"rerequested"}`), &back); err != nil || back.Outcome != component.Rerequested {
		t.Errorf("round trip: %q, %v", back.Outcome, err)
	}
}
