package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestSetupSavesThePricingForm(t *testing.T) {
	sandbox(t)
	resetCost(t)
	h, token := setupMux(t)
	script := filepath.Join(t.TempDir(), "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"models\":{\"s\":{\"in\":1,\"out\":2}}}'\n"), 0o700)
	body := `{"token":"` + token + `","answers":{"billing":"auto"},"pricing":{
	  "ttl":"48h","file":"","url":"https://prices.example.com/p.json","header_env":"PRICES_AUTH",
	  "translate":"` + script + `","timeout":"20s",
	  "prices":{"acme-large":{"in":3,"out":15,"tokenizer":"claude"}}}}`
	w := postSetup(h, body)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	p := readPrefs().Pricing
	if p.TTL != "48h" || p.URL != "https://prices.example.com/p.json" || p.HeaderEnv != "PRICES_AUTH" ||
		p.Translate != script || p.Timeout != "20s" || len(p.Prices) != 1 || p.Billing != "auto" {
		t.Fatalf("prefs = %+v", p)
	}
}

func TestSetupPricingFormIsAllOrNothingAndNeedsAToken(t *testing.T) {
	sandbox(t)
	h, token := setupMux(t)
	for name, pricing := range map[string]string{
		"bad ttl":      `{"ttl":"soon"}`,
		"bad url":      `{"url":"ftp://x/y"}`,
		"bad command":  `{"translate":"\"open"}`,
		"bad timeout":  `{"timeout":"forever"}`,
		"negative":     `{"prices":{"x":{"in":-1,"out":1}}}`,
		"unknown key":  `{"command":"id"}`,
		"header value": `{"header_env":"Bearer abc def"}`,
	} {
		w := postSetup(h, `{"token":"`+token+`","answers":{"billing":"api"},"pricing":`+pricing+`}`)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", name, w.Code)
		}
	}
	if p := readPrefs().Pricing; p.Billing != "" || p.TTL != "" || p.URL != "" {
		t.Fatalf("a refused form saved something: %+v", p)
	}
	w := postSetup(h, `{"token":"wrong","pricing":{"translate":"/bin/sh -c id"}}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("wrong token: %d", w.Code)
	}
	if p := readPrefs().Pricing; p.Translate != "" {
		t.Fatalf("a command was saved without a token: %+v", p)
	}
}

func TestSetupViewCarriesThePricingAndTheDetection(t *testing.T) {
	sandbox(t)
	v := buildSetupView(prefs{Pricing: pricingPrefs{TTL: "12h", URL: "https://x/y.json", Translate: "tr a"}})
	if v.Pricing == nil || v.Pricing.TTL != "12h" || v.Pricing.URL != "https://x/y.json" || v.Pricing.Translate != "tr a" {
		t.Fatalf("pricing = %+v", v.Pricing)
	}
	if len(v.Pricing.Detected) < 3 {
		t.Fatalf("detected = %+v", v.Pricing.Detected)
	}
	fresh := buildSetupView(prefs{})
	if fresh.Pricing == nil || fresh.Pricing.TTL != "24h" {
		t.Fatalf("defaults = %+v", fresh.Pricing)
	}
}

func TestSetupPricingTestRunsTheFormWithoutSavingAndKeepsTheToken(t *testing.T) {
	sandbox(t)
	resetCost(t)
	h, token := setupMux(t)
	script := filepath.Join(t.TempDir(), "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"models\":{\"probe\":{\"in\":1,\"out\":2}}}'\n"), 0o700)
	w := postSetupPath(h, "/api/setup/pricing-test", `{"token":"`+token+`","pricing":{"translate":"`+script+`"}}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "probe") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if p := readPrefs().Pricing; p.Translate != "" {
		t.Fatalf("test saved the command: %+v", p)
	}
	if !validSetupToken(token) {
		t.Fatal("test used up the token")
	}
	if w := postSetupPath(h, "/api/setup/pricing-test", `{"token":"nope","pricing":{"translate":"`+script+`"}}`); w.Code != http.StatusForbidden {
		t.Fatalf("no token: %d", w.Code)
	}
	bad := postSetupPath(h, "/api/setup/pricing-test", `{"token":"`+token+`","pricing":{"translate":"/nonexistent/x"}}`)
	if bad.Code != http.StatusOK || !strings.Contains(bad.Body.String(), `"error"`) {
		t.Fatalf("a failing script is reported, not a 500: %d %s", bad.Code, bad.Body)
	}
}

func postSetupPath(h http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Host = "127.0.0.1:7878"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
