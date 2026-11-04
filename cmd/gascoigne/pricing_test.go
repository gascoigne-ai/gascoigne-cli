package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

func resetCost(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { cost.SetOverrides(nil); cost.SetBilling("") })
}

func TestPricingSetSavesAndLoads(t *testing.T) {
	sandbox(t)
	resetCost(t)
	err := cmdPricing(context.Background(), []string{"set", "-billing", "api", "-ttl", "12h",
		"-price", "acme-large=3,15,0.3,3.75,claude", "-price", "acme-small=0.5,1"})
	if err != nil {
		t.Fatal(err)
	}
	p := readPrefs().Pricing
	if p.TTL != "12h" || len(p.Prices) != 2 || *p.Prices["acme-large"].In != 3 || p.Prices["acme-large"].Tokenizer != "claude" {
		t.Fatalf("prefs = %+v", p)
	}
	applyPricing(false)
	if m, source := cost.LookupSource("acme-large"); m.Input != 3 || m.CacheRead != 0.3 || source != "static" {
		t.Fatalf("price not in force: %+v %q", m, source)
	}
	// Unset one, then clear all.
	if err := cmdPricing(context.Background(), []string{"set", "-unset-price", "acme-small"}); err != nil {
		t.Fatal(err)
	}
	if got := readPrefs().Pricing.Prices; len(got) != 1 {
		t.Fatalf("after unset: %+v", got)
	}
	if err := cmdPricing(context.Background(), []string{"set", "-clear"}); err != nil {
		t.Fatal(err)
	}
	if got := readPrefs().Pricing; got.TTL != "" || len(got.Prices) != 0 {
		t.Fatalf("after clear: %+v", got)
	}
}

func TestPricingSetRefusesBadInputAndSavesNothing(t *testing.T) {
	sandbox(t)
	for _, args := range [][]string{
		{"set", "-billing", "free"},
		{"set", "-ttl", "soon"},
		{"set", "-price", "x=1"},
		{"set", "-price", "x=-1,2"},
		{"set", "-price", "x=1,2,3,4,bpe9"},
		{"set", "-price", "=1,2"},
		{"set", "-url", "ftp://example.com/p.json"},
		{"set", "-translate", `"unterminated`},
		{"set", "-timeout", "forever"},
		{"set", "-billing", "subscription", "-ttl", "soon"}, // all or nothing
	} {
		if err := cmdPricing(context.Background(), args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	if p := readPrefs().Pricing; p.Billing != "" || p.TTL != "" || len(p.Prices) != 0 {
		t.Fatalf("a refused set saved something: %+v", p)
	}
}

func TestPricingTTLBelowAnHourIsRaisedWithWarning(t *testing.T) {
	sandbox(t)
	var out bytes.Buffer
	if err := runPricingSet(&out, []string{"-ttl", "10m"}); err != nil {
		t.Fatal(err)
	}
	if readPrefs().Pricing.TTL != "1h0m0s" || !strings.Contains(out.String(), "using 1h") {
		t.Fatalf("ttl %q, output %q", readPrefs().Pricing.TTL, out.String())
	}
}

func TestSubscriptionBillingHidesDollars(t *testing.T) {
	sandbox(t)
	resetCost(t)
	if err := cmdPricing(context.Background(), []string{"set", "-billing", "subscription"}); err != nil {
		t.Fatal(err)
	}
	applyPricing(false)
	if cost.Billing() != cost.BillingSubscription {
		t.Fatal("billing not applied")
	}
}

func TestPricingTranslateComesOnlyFromTheUsersPrefs(t *testing.T) {
	sandbox(t)
	resetCost(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "t.sh")
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"models\":{\"s-model\":{\"in\":1,\"out\":2}}}'\n"), 0o700)
	if err := cmdPricing(context.Background(), []string{"set", "-translate", script}); err != nil {
		t.Fatal(err)
	}
	if err := cmdPricing(context.Background(), []string{"refresh"}); err != nil {
		t.Fatal(err)
	}
	applyPricing(false)
	if m, source := cost.LookupSource("s-model"); m.Input != 1 || source != "script" {
		t.Fatalf("script prices not in force: %+v %q", m, source)
	}
	// test runs without touching the cache the real run uses.
	os.WriteFile(script, []byte("#!/bin/sh\nprintf '{\"models\":{\"other\":{\"in\":9,\"out\":9}}}'\n"), 0o700)
	var out bytes.Buffer
	if err := runPricingTest(t.Context(), &out); err != nil || !strings.Contains(out.String(), "other") {
		t.Fatalf("test output %q, %v", out.String(), err)
	}
	applyPricing(false)
	if _, source := cost.LookupSource("other"); source != cost.SourceBuiltIn {
		t.Fatal("test wrote the real cache")
	}
}
