package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPricingSetSavesEverySetting(t *testing.T) {
	sandbox(t)
	var out bytes.Buffer
	args := []string{"-billing", "api", "-ttl", "6h", "-file", "p.json", "-url", "https://example.com/p.json",
		"-header-env", "PRICE_AUTH", "-translate", "convert --fast", "-timeout", "5s",
		"-price", "Alpha=1,2", "-price", "beta=3,4,0.5,6,o200k"}
	if err := runPricingSet(&out, args); err != nil {
		t.Fatal(err)
	}
	got := readPrefs().Pricing
	if got.Billing != "api" || got.TTL != "6h" || got.File != "p.json" || got.URL != "https://example.com/p.json" ||
		got.HeaderEnv != "PRICE_AUTH" || got.Translate != "convert --fast" || got.Timeout != "5s" {
		t.Errorf("prefs %+v", got)
	}
	if len(got.Prices) != 2 || *got.Prices["alpha"].In != 1 || *got.Prices["beta"].CacheWrite != 6 || got.Prices["beta"].Tokenizer != "o200k" {
		t.Errorf("prices %+v", got.Prices)
	}
	if !strings.Contains(out.String(), "pricing settings saved") {
		t.Errorf("output %q", out.String())
	}

	// A later set keeps what it does not name, drops unset prices, and
	// "manual" is a ttl of its own.
	if err := runPricingSet(&out, []string{"-ttl", "manual", "-unset-price", " ALPHA "}); err != nil {
		t.Fatal(err)
	}
	got = readPrefs().Pricing
	if got.TTL != "manual" || got.Billing != "api" || len(got.Prices) != 1 || got.Prices["beta"].Tokenizer != "o200k" {
		t.Errorf("after second set %+v", got)
	}
	if err := runPricingSet(&out, []string{"-unset-price", "beta"}); err != nil {
		t.Fatal(err)
	}
	if got := readPrefs().Pricing; got.Prices != nil {
		t.Errorf("no prices left should be nil: %+v", got.Prices)
	}
	if err := runPricingSet(&out, []string{"-clear"}); err != nil {
		t.Fatal(err)
	}
	if got := readPrefs().Pricing; got.Billing != "" || got.URL != "" {
		t.Errorf("clear left %+v", got)
	}
}
