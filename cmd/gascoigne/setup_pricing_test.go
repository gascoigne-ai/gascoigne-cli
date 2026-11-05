package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// setupMux serves the setup routes with a valid token and returns the token.
func setupMux(t *testing.T) (http.Handler, string) {
	t.Helper()
	mux := http.NewServeMux()
	setupHandlers(mux)
	token := "tok-" + t.Name()
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	put(t, setupTokenFile(), `{"token":"`+token+`","expires":"2099-01-01T00:00:00Z"}`)
	return localOnly(mux), token
}

func postSetupPath(h http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Host = "127.0.0.1:7878"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// The setup token is minted by a login-less endpoint, so anything a token holder can post must be
// harmless: billing, the refresh interval and static prices. A price file, URL, header variable or
// command would read files, reach the network or run a program for any local user.
func TestSetupSavesBillingIntervalAndPricesAndKeepsTheTerminalSources(t *testing.T) {
	sandbox(t)
	resetCost(t)
	prefsWithSources := readPrefs()
	prefsWithSources.Pricing = pricingPrefs{
		File: "/srv/prices.json", URL: "https://prices.example.com/p.json", HeaderEnv: "PRICES_AUTH",
		Translate: "/usr/local/bin/to-gascoigne", Timeout: "20s",
	}
	if err := prefsWithSources.save(); err != nil {
		t.Fatal(err)
	}
	h, token := setupMux(t)
	w := postSetup(h, `{"token":"`+token+`","answers":{"billing":"auto"},"pricing":{
	  "ttl":"48h","prices":{"acme-large":{"in":3,"out":15,"tokenizer":"claude"}}}}`)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	p := readPrefs().Pricing
	if p.TTL != "48h" || len(p.Prices) != 1 || p.Billing != "auto" {
		t.Fatalf("form not saved: %+v", p)
	}
	if p.File != "/srv/prices.json" || p.URL != "https://prices.example.com/p.json" || p.HeaderEnv != "PRICES_AUTH" ||
		p.Translate != "/usr/local/bin/to-gascoigne" || p.Timeout != "20s" {
		t.Fatalf("saving the form changed the sources set in the terminal: %+v", p)
	}
}

func TestSetupRefusesToSetWhatReadsFilesReachesTheNetworkOrRunsPrograms(t *testing.T) {
	sandbox(t)
	h, token := setupMux(t)
	for name, pricing := range map[string]string{
		"command":    `{"translate":"/bin/sh -c id"}`,
		"url":        `{"url":"https://attacker.example/p.json"}`,
		"file":       `{"file":"/etc/passwd"}`,
		"header env": `{"header_env":"AWS_SECRET_ACCESS_KEY"}`,
		"timeout":    `{"timeout":"1s"}`,
	} {
		w := postSetup(h, `{"token":"`+token+`","answers":{"billing":"api"},"pricing":`+pricing+`}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, w.Code)
		}
	}
	if p := readPrefs().Pricing; p.Billing != "" || p.Translate != "" || p.URL != "" || p.File != "" || p.HeaderEnv != "" {
		t.Fatalf("a refused form saved something: %+v", p)
	}
	// The route that ran the sources for the page is gone.
	w := postSetupPath(h, "/api/setup/pricing-test", `{"token":"`+token+`","pricing":{"ttl":"24h"}}`)
	if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("pricing-test still answers: %d", w.Code)
	}
}

func TestSetupPricingFormIsAllOrNothingAndNeedsAToken(t *testing.T) {
	sandbox(t)
	h, token := setupMux(t)
	for name, pricing := range map[string]string{
		"bad ttl":     `{"ttl":"soon"}`,
		"negative":    `{"prices":{"x":{"in":-1,"out":1}}}`,
		"missing out": `{"prices":{"x":{"in":1}}}`,
		"unknown key": `{"command":"id"}`,
	} {
		w := postSetup(h, `{"token":"`+token+`","answers":{"billing":"api"},"pricing":`+pricing+`}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, w.Code)
		}
	}
	if p := readPrefs().Pricing; p.Billing != "" || p.TTL != "" {
		t.Fatalf("a refused form saved something: %+v", p)
	}
	if w := postSetup(h, `{"token":"wrong","pricing":{"ttl":"12h"}}`); w.Code != http.StatusForbidden {
		t.Fatalf("wrong token: %d", w.Code)
	}
	if p := readPrefs().Pricing; p.TTL != "" {
		t.Fatalf("saved without a token: %+v", p)
	}
}

func TestSetupViewShowsWhichSourcesAreSetButNotWhatTheyAre(t *testing.T) {
	sandbox(t)
	view := buildSetupView(prefs{Pricing: pricingPrefs{
		TTL: "12h", URL: "https://x.example/y.json?token=zq9marker", Translate: "tr --key zq9marker",
	}})
	if view.Pricing == nil || view.Pricing.TTL != "12h" || len(view.Pricing.Detected) < 3 {
		t.Fatalf("pricing = %+v", view.Pricing)
	}
	if !view.Pricing.Sources.URL || !view.Pricing.Sources.Translate || view.Pricing.Sources.File {
		t.Fatalf("sources = %+v", view.Pricing.Sources)
	}
	if body := mustJSON(t, view); strings.Contains(body, "zq9marker") {
		t.Fatalf("the setup view leaks a URL or command line: %s", body)
	}
	if fresh := buildSetupView(prefs{}); fresh.Pricing == nil || fresh.Pricing.TTL != "24h" {
		t.Fatalf("defaults = %+v", fresh.Pricing)
	}
}
