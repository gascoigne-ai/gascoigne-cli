package mcptool

import (
	"context"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// agentLedger: claude runs with and without a recorded model, and a model
// gascoigne has no price for.
type agentLedger struct{ component.Ledger }

func (agentLedger) Stats(_ context.Context, q component.StatsQuery) ([]component.Stat, error) {
	switch {
	case q.By == component.ByAgent:
		return []component.Stat{{Group: "claude"}, {Group: "pi"}}, nil
	case q.By == component.ByModel && q.Agent == "claude":
		return []component.Stat{
			{Group: "claude-sonnet-5-5", RawTokens: 3000, OutTokens: 1000},
			{Group: "-", RawTokens: 2000, OutTokens: 1000},
		}, nil
	case q.By == component.ByModel && q.Agent == "pi":
		return []component.Stat{{Group: "local-llama", RawTokens: 500, OutTokens: 100}}, nil
	}
	return nil, nil
}

func TestBuildCosts(t *testing.T) {
	c, err := BuildCosts(t.Context(), agentLedger{}, component.StatsQuery{}, cost.Context{Rereads: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Savings) != 2 || len(c.Unpriced) != 1 || c.Unpriced[0] != "local-llama" {
		t.Fatalf("got %+v", c)
	}
	byModel := map[string]cost.Saving{}
	for _, s := range c.Savings {
		byModel[s.Model] = s
	}
	if s := byModel["claude-sonnet-5-5"]; s.Tokens != 2000 || s.Assumed {
		t.Errorf("recorded model: %+v", s)
	}
	if s := byModel["claude-opus-5-5"]; s.Tokens != 1000 || !s.Assumed {
		t.Errorf("claude without a model should assume its default: %+v", s)
	}
	if c.InContext != c.FirstRead {
		t.Errorf("no rereads, yet in context %v != first read %v", c.InContext, c.FirstRead)
	}
}

// evalLedger: recorded eval runs (agent "eval") beside a real agent. Eval runs
// are synthetic, so they are neither priced nor listed as an unpriced agent.
type evalLedger struct{ component.Ledger }

func (evalLedger) Stats(_ context.Context, q component.StatsQuery) ([]component.Stat, error) {
	switch {
	case q.By == component.ByAgent:
		return []component.Stat{{Group: "claude"}, {Group: "eval"}}, nil
	case q.By == component.ByModel && q.Agent == "claude":
		return []component.Stat{{Group: "claude-sonnet-5-5", RawTokens: 3000, OutTokens: 1000}}, nil
	case q.By == component.ByModel && q.Agent == "eval":
		return []component.Stat{{Group: "-", RawTokens: 9000, OutTokens: 1000}}, nil
	}
	return nil, nil
}

func TestBuildCostsSkipsEvalRuns(t *testing.T) {
	c, err := BuildCosts(t.Context(), evalLedger{}, component.StatsQuery{}, cost.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Unpriced) != 0 {
		t.Errorf("eval listed as unpriced: %v", c.Unpriced)
	}
	if len(c.Savings) != 1 || c.Savings[0].Tokens != 2000 {
		t.Errorf("savings %+v", c.Savings)
	}
}

func TestBuildCostsUsesTheUsersPriceAndSaysWhere(t *testing.T) {
	t.Cleanup(func() { cost.SetOverrides(nil); cost.SetBilling("") })
	cost.SetOverrides([]cost.Override{
		{Model: cost.Model{ID: "local-llama", Tokenizer: "o200k", Input: 1, Output: 2, CacheWrite: 1, CacheRead: 1}, Source: "static"},
		{Model: cost.Model{ID: "claude-sonnet-5-5", Tokenizer: "claude", Input: 4, Output: 20, CacheWrite: 5, CacheRead: 0.4}, Source: "json"},
	})
	c, err := BuildCosts(t.Context(), agentLedger{}, component.StatsQuery{}, cost.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Unpriced) != 0 {
		t.Fatalf("a model with a custom price is priced: %v", c.Unpriced)
	}
	sources := map[string]string{}
	for _, s := range c.Savings {
		sources[s.Model] = s.Source
	}
	want := map[string]string{"local-llama": "static", "claude-sonnet-5-5": "json", "claude-opus-5-5": cost.SourceBuiltIn}
	if len(sources) != len(want) || sources["local-llama"] != "static" || sources["claude-sonnet-5-5"] != "json" || sources["claude-opus-5-5"] != cost.SourceBuiltIn {
		t.Fatalf("sources = %v, want %v", sources, want)
	}
	if c.Billing != cost.BillingAPI {
		t.Fatalf("billing = %q", c.Billing)
	}
}

func TestBuildCostsUnderSubscriptionComputesNoDollars(t *testing.T) {
	t.Cleanup(func() { cost.SetBilling("") })
	cost.SetBilling(cost.BillingSubscription)
	c, err := BuildCosts(t.Context(), agentLedger{}, component.StatsQuery{}, cost.AgentContext)
	if err != nil {
		t.Fatal(err)
	}
	if c.Billing != cost.BillingSubscription || len(c.Savings) != 0 || c.InContext != 0 || c.FirstRead != 0 || len(c.Unpriced) != 0 {
		t.Fatalf("got %+v", c)
	}
	if c.Savings == nil {
		t.Error("savings must encode as [] not null")
	}
	if c.Pricing.Basis == "" {
		t.Error("the page needs a sentence saying why there are no dollars")
	}
}

func TestBuildCostsSkipsSubscriptionAgentsAndKeepsAPIOnes(t *testing.T) {
	t.Cleanup(func() { cost.SetBilling(""); cost.SetDetector(nil) })
	cost.SetDetector(func(agent string) cost.Detection {
		if agent == "claude" {
			return cost.Detection{Mode: cost.BillingSubscription, Why: "signed in"}
		}
		return cost.Detection{Mode: cost.BillingAPI, Why: "key"}
	})
	c, err := BuildCosts(t.Context(), agentLedger{}, component.StatsQuery{}, cost.Context{})
	if err != nil {
		t.Fatal(err)
	}
	// claude is on a plan: its savings are not priced; pi (API) still is not priced here
	// because local-llama has no price, but it is listed.
	if len(c.Savings) != 0 || len(c.Unpriced) != 1 || c.Unpriced[0] != "local-llama" {
		t.Fatalf("got %+v", c)
	}
	if len(c.SubscriptionAgents) != 1 || c.SubscriptionAgents[0] != "claude" {
		t.Fatalf("subscription agents = %v", c.SubscriptionAgents)
	}
	if c.Billing != "mixed" {
		t.Fatalf("billing = %q, want mixed", c.Billing)
	}
}

func TestBuildCostsAllAgentsOnAPlanIsSubscription(t *testing.T) {
	t.Cleanup(func() { cost.SetBilling(""); cost.SetDetector(nil) })
	cost.SetDetector(func(string) cost.Detection { return cost.Detection{Mode: cost.BillingSubscription} })
	c, err := BuildCosts(t.Context(), agentLedger{}, component.StatsQuery{}, cost.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Billing != cost.BillingSubscription || len(c.Savings) != 0 || len(c.Unpriced) != 0 || c.InContext != 0 {
		t.Fatalf("got %+v", c)
	}
	if len(c.SubscriptionAgents) != 2 {
		t.Fatalf("subscription agents = %v", c.SubscriptionAgents)
	}
}
