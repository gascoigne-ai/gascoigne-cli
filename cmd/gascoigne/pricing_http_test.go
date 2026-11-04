package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pricingCall(t *testing.T, method, path, body string) (*httptest.ResponseRecorder, pricingView) {
	t.Helper()
	mux := http.NewServeMux()
	pricingHandlers(mux)
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Host = "127.0.0.1:7878"
	recorder := httptest.NewRecorder()
	localOnly(mux).ServeHTTP(recorder, request)
	var view pricingView
	if recorder.Code == http.StatusOK {
		if err := json.NewDecoder(recorder.Body).Decode(&view); err != nil {
			t.Fatal(err)
		}
	}
	return recorder, view
}

// The dashboard has no login: it must not change a setting, and above all not the ones that read
// files, reach the network or run programs.
func TestPricingAPIChangesNoSetting(t *testing.T) {
	sandbox(t)
	resetCost(t)
	for _, body := range []string{
		`{"billing":"subscription"}`, `{"ttl":"48h"}`, `{"prices":{"x":{"in":1,"out":2}}}`,
		`{"translate":"/bin/sh -c id"}`, `{"url":"http://attacker.invalid/"}`, `{"file":"/etc/passwd"}`,
		`{"header_env":"HOME"}`,
	} {
		rec, _ := pricingCall(t, "POST", "/api/pricing", body)
		if rec.Code != http.StatusMethodNotAllowed && rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, the dashboard must not accept settings", body, rec.Code)
		}
	}
	if p := readPrefs().Pricing; p.Billing != "" || p.Translate != "" || p.URL != "" || len(p.Prices) != 0 {
		t.Fatalf("settings were written: %+v", p)
	}
}

func TestPricingAPIShowsSourcesAndRunsNow(t *testing.T) {
	sandbox(t)
	resetCost(t)
	script := filepath.Join(t.TempDir(), "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"models\":{\"s-model\":{\"in\":1,\"out\":2}}}'\n"), 0o700)
	if err := cmdPricing(t.Context(), []string{"set", "-translate", script}); err != nil {
		t.Fatal(err)
	}
	_, before := pricingCall(t, "GET", "/api/pricing", "")
	if !before.Remote || before.TranslateSet != true || len(before.Layers) != 1 || before.Layers[0].Name != "script" || before.Layers[0].Models != 0 {
		t.Fatalf("before = %+v", before)
	}
	if strings.Contains(mustJSON(t, before), script) {
		t.Error("the view leaks the command line")
	}
	rec, after := pricingCall(t, "POST", "/api/pricing/refresh", "")
	if rec.Code != http.StatusOK || len(after.Layers) != 1 || after.Layers[0].Models != 1 || after.Layers[0].Error != "" {
		t.Fatalf("after = %d %+v", rec.Code, after)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
