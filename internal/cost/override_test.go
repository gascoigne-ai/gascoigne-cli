package cost

import (
	"strings"
	"testing"
)

func TestOverridesBeatBuiltInAndKeepSource(t *testing.T) {
	t.Cleanup(func() { SetOverrides(nil) })
	if _, src := LookupSource("claude-sonnet-4-5"); src != SourceBuiltIn {
		t.Fatalf("source = %q", src)
	}
	SetOverrides([]Override{
		{Model: Model{ID: "claude-sonnet-4-5", Tokenizer: "claude-legacy", Input: 1, Output: 2, CacheWrite: 1, CacheRead: 0.5}, Source: "static"},
		{Model: Model{ID: "acme-large", Tokenizer: "o200k", Input: 7, Output: 8, CacheWrite: 7, CacheRead: 7}, Source: "json"},
	})
	m, src := LookupSource("claude-sonnet-4-5-20251001") // dated id still finds the override
	if src != "static" || m.Input != 1 || m.CacheRead != 0.5 {
		t.Fatalf("override ignored: %+v from %q", m, src)
	}
	if m, ok := Lookup("ACME-large"); !ok || m.Input != 7 {
		t.Fatalf("custom model not priced: %+v %v", m, ok)
	}
	if _, src := LookupSource("claude-haiku-5-5"); src != SourceBuiltIn {
		t.Fatalf("untouched model source = %q", src)
	}
	SetOverrides(nil)
	if m, _ := Lookup("claude-sonnet-4-5"); m.Input != 3 {
		t.Fatalf("clearing overrides must restore the built-in price: %+v", m)
	}
}

func TestLongerBuiltInPrefixStillBeatsShorterOverride(t *testing.T) {
	t.Cleanup(func() { SetOverrides(nil) })
	SetOverrides([]Override{{Model: Model{ID: "claude-opus-4", Tokenizer: "claude-legacy", Input: 1, Output: 1, CacheWrite: 1, CacheRead: 1}, Source: "static"}})
	// claude-opus-4-1 is a longer built-in id: the longest match wins, as for built-ins alone.
	if m, src := LookupSource("claude-opus-4-1"); src != SourceBuiltIn || m.Input != 15 {
		t.Fatalf("got %+v from %q", m, src)
	}
}

func TestParsePrices(t *testing.T) {
	good := `{"version": 1, "models": {
		"acme-large": {"in": 3, "out": 15, "cache_read": 0.3, "cache_write": 3.75, "tokenizer": "claude"},
		"acme-small": {"in": 0.5, "out": 1}
	}}`
	got, err := ParsePrices([]byte(good), "json")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Model.ID != "acme-large" || got[1].Model.ID != "acme-small" {
		t.Fatalf("models sorted by id: %+v", got)
	}
	small := got[1].Model
	if small.CacheWrite != 0.5 || small.CacheRead != 0.5 || small.Tokenizer != "o200k" || got[1].Source != "json" {
		t.Fatalf("defaults wrong: %+v", got[1])
	}
	for name, bad := range map[string]string{
		"not json":        `{`,
		"no models":       `{"version": 1}`,
		"missing out":     `{"models": {"x": {"in": 1}}}`,
		"negative":        `{"models": {"x": {"in": -1, "out": 1}}}`,
		"unknown field":   `{"models": {"x": {"in": 1, "out": 1, "cost": 2}}}`,
		"unknown tok":     `{"models": {"x": {"in": 1, "out": 1, "tokenizer": "bpe9"}}}`,
		"empty id":        `{"models": {"": {"in": 1, "out": 1}}}`,
		"future version":  `{"version": 99, "models": {"x": {"in": 1, "out": 1}}}`,
		"string price":    `{"models": {"x": {"in": "1", "out": 1}}}`,
		"trailing object": `{"models": {"x": {"in": 1, "out": 1}}} {}`,
	} {
		if _, err := ParsePrices([]byte(bad), "json"); err == nil {
			t.Errorf("%s accepted", name)
		} else if !strings.Contains(err.Error(), "json") {
			t.Errorf("%s: error does not name the source: %v", name, err)
		}
	}
}

func TestBillingForcedOrDetectedPerAgent(t *testing.T) {
	t.Cleanup(func() { SetBilling(""); SetDetector(nil) })
	SetDetector(func(agent string) Detection {
		if agent == "claude" {
			return Detection{Mode: BillingSubscription, Why: "signed in"}
		}
		return Detection{Mode: BillingAPI, Why: "key"}
	})
	SetBilling("")
	if Billing() != BillingAuto {
		t.Fatalf("default = %q, want auto", Billing())
	}
	if got := BillingOf("claude"); got.Mode != BillingSubscription || got.Why != "signed in" {
		t.Fatalf("claude = %+v", got)
	}
	if got := BillingOf("codex"); got.Mode != BillingAPI {
		t.Fatalf("codex = %+v", got)
	}
	SetBilling(BillingAPI) // forced: detection is not asked
	if got := BillingOf("claude"); got.Mode != BillingAPI || got.Why != "set by you" {
		t.Fatalf("forced api = %+v", got)
	}
	SetBilling(BillingSubscription)
	if got := BillingOf("codex"); got.Mode != BillingSubscription {
		t.Fatalf("forced subscription = %+v", got)
	}
	SetBilling("nonsense") // unknown values read as auto
	if Billing() != BillingAuto {
		t.Fatalf("unknown = %q", Billing())
	}
	SetDetector(nil)
	if got := BillingOf("anything"); got.Mode != BillingAPI {
		t.Fatalf("no detector: %+v", got)
	}
}
