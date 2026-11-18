package main

import (
	"cmp"
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pricing"
)

// pricingView is what the Costs page shows and edits. Files, URLs and commands are not in it
// beyond "set": see pricingHandlers.
type pricingView struct {
	Billing  string                    `json:"billing"`
	Detected []detected                `json:"detected"`
	TTL      string                    `json:"ttl"`
	Prices   map[string]cost.PriceItem `json:"prices"`
	Layers   []layerView               `json:"layers"`
	Warnings []string                  `json:"warnings,omitempty"`
	// Remote says a file, URL or command is set (with the CLI), so "run now" has something to do.
	Remote       bool   `json:"remote"`
	FileSet      bool   `json:"file_set,omitempty"`
	URLSet       bool   `json:"url_set,omitempty"`
	TranslateSet bool   `json:"translate_set,omitempty"`
	Checked      string `json:"builtin_checked"`
	Error        string `json:"error,omitempty"`
}

type layerView struct {
	Name    string    `json:"name"`
	Models  int       `json:"models"`
	Fetched time.Time `json:"fetched,omitzero"`
	Next    time.Time `json:"next,omitzero"`
	Error   string    `json:"error,omitempty"`
}

func buildPricingView() pricingView {
	p := readPrefs().Pricing
	cfg, warnings := p.config()
	result := pricing.Load(cfg)
	view := pricingView{
		Billing: cmp.Or(p.Billing, cost.BillingAuto), Detected: detectedAgents(), TTL: cmp.Or(p.TTL, "24h"), Prices: p.Prices, Warnings: warnings,
		Layers: []layerView{}, Checked: cost.PricesChecked,
		FileSet: p.File != "", URLSet: p.URL != "", TranslateSet: p.Translate != "",
		Remote: p.URL != "" || p.Translate != "",
	}
	if view.Prices == nil {
		view.Prices = map[string]cost.PriceItem{}
	}
	for _, layer := range result.Layers {
		view.Layers = append(view.Layers, layerView{Name: layer.Name, Models: layer.Models, Fetched: layer.Fetched, Next: layer.Next, Error: layer.Error})
	}
	return view
}

// pricingHandlers serves GET /api/pricing (what the Costs page shows) and POST
// /api/pricing/refresh (run the configured sources now). The dashboard has no login, so nothing
// here changes a setting: a file, URL, header variable or command would let any local process
// read files, send the server's environment to a host or run a program. Settings are changed on
// the setup page (behind its one-time token) or with gascoigne pricing set.
func pricingHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/pricing", func(writer http.ResponseWriter, _ *http.Request) {
		respond(writer, buildPricingView(), nil)
	})
	mux.HandleFunc("POST /api/pricing/refresh", func(writer http.ResponseWriter, request *http.Request) {
		cfg, _ := readPrefs().Pricing.config()
		ctx, cancel := context.WithTimeout(request.Context(), 3*cfg.Timeout+30*time.Second)
		defer cancel()
		err := pricing.Refresh(ctx, cfg, true)
		applyPricing(false)
		view := buildPricingView()
		if err != nil {
			view.Error = err.Error()
		}
		respond(writer, view, nil)
	})
}

// watchPricing keeps a long-running server's prices fresh: each tick runs what is due and
// puts the result in force.
func watchPricing(ctx context.Context, every time.Duration) {
	applyPricing(false)
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg, _ := readPrefs().Pricing.config()
			pricing.Refresh(ctx, cfg, false)
			applyPricing(false)
		}
	}
}

// pricingForm is the pricing part of the setup page, as it posts it: the refresh interval and the
// static prices, nothing else. The setup token is minted by an endpoint with no login, so a form
// may not name what reads a file, reaches the network or runs a program (a price file, URL, header
// variable, command or time limit): those are set only with gascoigne pricing set, and a form that
// names one is refused (unknown fields are a 400).
type pricingForm struct {
	TTL    string                    `json:"ttl"`
	Prices map[string]cost.PriceItem `json:"prices"`
}

// apply validates the form and writes it over p's interval and static prices; the sources set in the
// terminal stay as they are.
func (f pricingForm) apply(chosen *pricingPrefs) error {
	duration, manual, warning, err := pricing.ParseTTL(f.TTL)
	if err != nil {
		return err
	}
	if err := validateStatic(f.Prices); err != nil {
		return err
	}
	switch {
	case manual:
		chosen.TTL = "manual"
	case warning != "":
		chosen.TTL = duration.String()
	default:
		chosen.TTL = strings.TrimSpace(f.TTL)
	}
	chosen.Prices = nil
	if len(f.Prices) > 0 {
		chosen.Prices = f.Prices
	}
	return nil
}

// pricingSources says which sources set in the terminal are in use. The values stay out of the
// setup view: a URL or a command line can carry a secret, and gascoigne pricing shows them.
type pricingSources struct {
	File      bool `json:"file"`
	URL       bool `json:"url"`
	HeaderEnv bool `json:"header_env"`
	Translate bool `json:"translate"`
}

// pricingSetup is the pricing form's current state, for the setup page.
type pricingSetup struct {
	pricingForm
	Sources  pricingSources `json:"sources"`
	Detected []detected     `json:"detected"`
	Layers   []layerView    `json:"layers"`
	Warnings []string       `json:"warnings,omitempty"`
}

func buildPricingSetup(chosen prefs) *pricingSetup {
	view := buildPricingView()
	out := &pricingSetup{
		pricingForm: pricingForm{TTL: cmp.Or(chosen.Pricing.TTL, "24h"), Prices: chosen.Pricing.Prices},
		Sources: pricingSources{
			File: chosen.Pricing.File != "", URL: chosen.Pricing.URL != "", HeaderEnv: chosen.Pricing.HeaderEnv != "",
			Translate: chosen.Pricing.Translate != "",
		},
		Detected: detectedAgents(), Layers: view.Layers, Warnings: view.Warnings,
	}
	if out.Prices == nil {
		out.Prices = map[string]cost.PriceItem{}
	}
	return out
}
