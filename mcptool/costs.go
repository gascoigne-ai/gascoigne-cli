package mcptool

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/cost"
)

// Costs is what the tokens saved are worth, per model.
type Costs struct {
	Savings []cost.Saving `json:"savings"`
	// FirstRead counts each saved token once; InContext adds the rereads
	// of every later request while the tool result stays in context.
	FirstRead float64 `json:"first_read_usd"`
	InContext float64 `json:"in_context_usd"`
	Rereads   float64 `json:"rereads"`
	// Unpriced are models (or agents without a model) with no known price.
	Unpriced []string `json:"unpriced,omitempty"`
}

// BuildCosts prices the savings of the runs matching filter, per agent and
// model; runs without a model are priced as the agent's default model.
func BuildCosts(ctx context.Context, l component.Ledger, filter component.StatsQuery, c cost.Context) (Costs, error) {
	out := Costs{Savings: []cost.Saving{}, Rereads: c.Rereads}
	q := filter
	q.By = component.ByAgent
	agents, err := l.Stats(ctx, q)
	if err != nil {
		return out, err
	}
	saved := map[[2]string]int{} // model, assumed → tokens
	for _, a := range agents {
		mq := filter
		mq.By, mq.Agent = component.ByModel, a.Group
		if a.Group == "-" {
			continue // runs without an agent: no default model to assume
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
		m, _ := cost.Lookup(k[0])
		s := cost.Estimate(m, n, c)
		s.Assumed = k[1] != ""
		out.Savings = append(out.Savings, s)
	}
	cost.Sort(out.Savings)
	out.FirstRead, out.InContext = cost.Total(out.Savings)
	slices.Sort(out.Unpriced)
	return out, nil
}
