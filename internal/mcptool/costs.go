package mcptool

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// Costs is what the tokens saved are worth, per model.
type Costs struct {
	// Billing is "api", "subscription" (every agent is on a plan: no dollars) or "mixed" (some
	// agents are on a plan, their savings are left out of the dollars).
	Billing string `json:"billing"`
	// SubscriptionAgents are the agents left out because they are billed by plan, not per token.
	SubscriptionAgents []string      `json:"subscription_agents,omitempty"`
	Savings            []cost.Saving `json:"savings"`
	// FirstRead counts each saved token once; InContext adds the rereads
	// of every later request while the tool result stays in context.
	FirstRead float64 `json:"first_read_usd"`
	InContext float64 `json:"in_context_usd"`
	Rereads   float64 `json:"rereads"`
	// Pricing says the dollar figures are estimates from the providers'
	// official list prices, not what an invoice shows.
	Pricing Pricing `json:"pricing"`
	// Unpriced are models (or agents without a model) with no known price.
	Unpriced []string `json:"unpriced,omitempty"`
}

// Pricing is where the prices behind Costs come from.
type Pricing struct {
	Basis   string   `json:"basis"`
	Checked string   `json:"checked"`
	Sources []string `json:"sources"`
}

// BuildCosts prices the savings of the runs matching filter, per agent and
// model; runs without a model are priced as the agent's default model.
func BuildCosts(ctx context.Context, l component.Ledger, filter component.StatsQuery, c cost.Context) (Costs, error) {
	out := Costs{Billing: cost.BillingAPI, Savings: []cost.Saving{}, Rereads: c.Rereads, Pricing: Pricing{
		Basis:   "estimate from the providers' official list prices (standard tier, short context); discounts, batch and priority tiers, and your plan are not applied",
		Checked: cost.PricesChecked,
		Sources: cost.PriceSources,
	}}
	if cost.Billing() == cost.BillingSubscription {
		out.Billing = cost.BillingSubscription
		out.Pricing = Pricing{Basis: "not shown: billing is set to a flat plan, so tokens saved are not turned into dollars (gascoigne pricing set -billing api)"}
		return out, nil
	}
	q := filter
	q.By = component.ByAgent
	agents, err := l.Stats(ctx, q)
	if err != nil {
		return out, err
	}
	saved := map[[2]string]int{} // model, assumed → tokens
	custom := false
	for _, a := range agents {
		mq := filter
		mq.By, mq.Agent = component.ByModel, a.Group
		if a.Group == "-" || a.Group == "eval" {
			continue // no agent: no default model to assume; eval: recorded benchmark runs, not work
		}
		if cost.BillingOf(a.Group).Mode == cost.BillingSubscription {
			out.SubscriptionAgents = append(out.SubscriptionAgents, a.Group)
			continue // a plan bills nothing per token
		}
		models, err := l.Stats(ctx, mq)
		if err != nil {
			return out, err
		}
		for _, m := range models {
			if m.Saved() <= 0 {
				continue
			}
			id, assumed := m.Group, "" // "" = recorded
			if id == "" || id == "-" { // the ledger's label for none
				id, assumed = cost.AgentModels[a.Group], "assumed"
			}
			model, ok := cost.Lookup(id)
			if !ok {
				name := cmp.Or(id, "agent "+cmp.Or(a.Group, "unknown"))
				if !slices.Contains(out.Unpriced, name) {
					out.Unpriced = append(out.Unpriced, name)
				}
				continue
			}
			saved[[2]string{model.ID, assumed}] += m.Saved()
		}
	}
	for _, k := range slices.SortedFunc(maps.Keys(saved), func(a, b [2]string) int {
		return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1]))
	}) {
		n := saved[k]
		m, source := cost.LookupSource(k[0])
		s := cost.Estimate(m, n, c)
		s.Assumed = k[1] != ""
		s.Source = source
		custom = custom || source != cost.SourceBuiltIn
		out.Savings = append(out.Savings, s)
	}
	if custom {
		out.Pricing.Basis += "; prices you supplied replace the list price where marked in source"
	}
	switch {
	case len(out.SubscriptionAgents) > 0 && len(out.Savings) == 0 && len(out.Unpriced) == 0:
		out.Billing = cost.BillingSubscription
		out.Pricing.Basis = "not shown: every agent with savings is billed by plan, so its tokens saved are not turned into dollars"
	case len(out.SubscriptionAgents) > 0:
		out.Billing = "mixed"
		out.Pricing.Basis += "; agents billed by plan (" + strings.Join(out.SubscriptionAgents, ", ") + ") are left out"
	}
	cost.Sort(out.Savings)
	out.FirstRead, out.InContext = cost.Total(out.Savings)
	slices.Sort(out.Unpriced)
	return out, nil
}
