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
