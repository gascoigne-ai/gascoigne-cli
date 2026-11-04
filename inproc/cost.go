package inproc

import (
	"context"
	"fmt"
	"os"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pricing"
)

// Billing modes for Pricing.Billing.
const (
	BillingAuto         = cost.BillingAuto
	BillingAPI          = cost.BillingAPI
	BillingSubscription = cost.BillingSubscription
)

// Pricing is where dollar figures come from. The zero value means the built-in list prices.
// Prices are held per process, not per Engine: the last Engine opened with a Pricing sets them.
// An embedder has no remote refresh or translator command: it passes the prices it wants.
type Pricing struct {
	// Billing is BillingAuto (default: an embedder has no sign-in to look at, so every agent is on
	// the API), BillingAPI or BillingSubscription, which shows no dollars.
	Billing string
	// Prices are static prices by model id; they beat File and the built-in list.
	Prices map[string]Price
	// File is a price sheet (JSON, see cost.ParsePrices), read once when the Engine opens.
	File string
}

// Price is one model's price in dollars per million tokens. In and Out are required. CacheRead
// and CacheWrite default to In; Tokenizer to "o200k" (also "claude", "claude-legacy", "gemini").
type Price struct {
	In         *float64 `json:"in"`
	Out        *float64 `json:"out"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
	Tokenizer  string   `json:"tokenizer,omitempty"`
}

// apply validates the prices and puts them in force.
func (p *Pricing) apply() error {
	if p == nil {
		return nil
	}
	if p.Billing != "" && p.Billing != BillingAuto && p.Billing != BillingAPI && p.Billing != BillingSubscription {
		return fmt.Errorf("pricing billing %q: want %q, %q or %q", p.Billing, BillingAuto, BillingAPI, BillingSubscription)
	}
	cfg := pricing.Config{}
	if len(p.Prices) > 0 {
		cfg.Static = make(map[string]cost.PriceItem, len(p.Prices))
		for id, price := range p.Prices {
			cfg.Static[id] = cost.PriceItem{In: price.In, Out: price.Out, CacheRead: price.CacheRead, CacheWrite: price.CacheWrite, Tokenizer: price.Tokenizer}
		}
	}
	if p.File != "" {
		if _, err := os.Stat(p.File); err != nil {
			return fmt.Errorf("pricing file: %w", err)
		}
		cfg.File = p.File
	}
	result := pricing.Load(cfg)
	for _, layer := range result.Layers {
		if layer.Error != "" {
			return fmt.Errorf("pricing %s: %s", layer.Name, layer.Error)
		}
	}
	cost.SetOverrides(result.Overrides)
	cost.SetBilling(p.Billing)
	return nil
}

// Costs is what the tokens saved are worth: estimates from list prices or the host's prices,
// not what anyone is billed. Under a subscription it has no dollars.
type Costs struct {
	Billing      string      `json:"billing"`
	FirstReadUSD float64     `json:"first_read_usd"`
	InContextUSD float64     `json:"in_context_usd"`
	Rereads      float64     `json:"rereads"`
	Savings      []ModelCost `json:"savings"`
	Unpriced     []string    `json:"unpriced,omitempty"`
}

// ModelCost is the saving priced for one model.
type ModelCost struct {
	Model        string  `json:"model"`
	Source       string  `json:"source"` // built-in, static or json
	Assumed      bool    `json:"assumed,omitempty"`
	Tokenizer    string  `json:"tokenizer"`
	Tokens       int     `json:"tokens"`       // saved, in gascoigne's estimate units
	ModelTokens  int     `json:"model_tokens"` // saved, in the model's own tokens
	FirstReadUSD float64 `json:"first_read_usd"`
	InContextUSD float64 `json:"in_context_usd"`
}

// Costs prices the savings of the runs matching q (By is ignored).
func (e *Engine) Costs(ctx context.Context, q StatsQuery) (Costs, error) {
	return CostsOf(ctx, e.led, q)
}

// CostsOf is Costs over any ledger, for servers that own one.
func CostsOf(ctx context.Context, led *ledger.Ledger, q StatsQuery) (Costs, error) {
	if led == nil {
		return Costs{}, ErrNoLedger
	}
	built, err := mcptool.BuildCosts(ctx, led, q.component(), cost.AgentContext)
	if err != nil {
		return Costs{}, err
	}
	out := Costs{
		Billing: built.Billing, FirstReadUSD: built.FirstRead, InContextUSD: built.InContext,
		Rereads: built.Rereads, Savings: []ModelCost{}, Unpriced: built.Unpriced,
	}
	for _, saving := range built.Savings {
		out.Savings = append(out.Savings, ModelCost{
			Model: saving.Model, Source: saving.Source, Assumed: saving.Assumed, Tokenizer: saving.Tokenizer,
			Tokens: saving.Tokens, ModelTokens: saving.ModelTokens,
			FirstReadUSD: saving.FirstRead, InContextUSD: saving.InContext,
		})
	}
	return out, nil
}
