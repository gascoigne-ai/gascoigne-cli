package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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
		Billing: cmpOr(p.Billing, cost.BillingAuto), Detected: detectedAgents(), TTL: cmpOr(p.TTL, "24h"), Prices: p.Prices, Warnings: warnings,
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
	mux.HandleFunc("GET /api/pricing", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, buildPricingView(), nil)
	})
	mux.HandleFunc("POST /api/pricing/refresh", func(w http.ResponseWriter, r *http.Request) {
		cfg, _ := readPrefs().Pricing.config()
		ctx, cancel := context.WithTimeout(r.Context(), 3*cfg.Timeout+30*time.Second)
		defer cancel()
		err := pricing.Refresh(ctx, cfg, true)
		applyPricing(false)
		view := buildPricingView()
		if err != nil {
			view.Error = err.Error()
		}
		respond(w, view, nil)
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

// pricingForm is the pricing part of the setup page, as it posts it. All fields are text; empty
// means unset. Unknown fields are refused.
type pricingForm struct {
	TTL       string                    `json:"ttl"`
	File      string                    `json:"file"`
	URL       string                    `json:"url"`
	HeaderEnv string                    `json:"header_env"`
	Translate string                    `json:"translate"`
	Timeout   string                    `json:"timeout"`
	Prices    map[string]cost.PriceItem `json:"prices"`
}

// prefs validates the form and turns it into settings (billing is a separate question).
func (f pricingForm) prefs() (pricingPrefs, error) {
	var p pricingPrefs
	duration, manual, warning, err := pricing.ParseTTL(f.TTL)
	if err != nil {
		return p, err
	}
	switch {
	case manual:
		p.TTL = "manual"
	case warning != "":
		p.TTL = duration.String()
	default:
		p.TTL = strings.TrimSpace(f.TTL)
	}
	if f.URL != "" && !strings.HasPrefix(f.URL, "https://") && !strings.HasPrefix(f.URL, "http://") {
		return p, fmt.Errorf("url wants an http or https address")
	}
	if f.Translate != "" {
		if _, err := pricing.SplitArgs(f.Translate); err != nil {
			return p, fmt.Errorf("command: %w", err)
		}
	}
	if f.Timeout != "" {
		if d, err := time.ParseDuration(f.Timeout); err != nil || d <= 0 {
			return p, fmt.Errorf("timeout %q: want a duration such as 10s", f.Timeout)
		}
	}
	if strings.ContainsAny(f.HeaderEnv, " \t:=") {
		return p, fmt.Errorf("header variable: give the name of an environment variable, not a value")
	}
	if err := validateStatic(f.Prices); err != nil {
		return p, err
	}
	p.File, p.URL, p.HeaderEnv, p.Translate, p.Timeout = f.File, f.URL, f.HeaderEnv, f.Translate, f.Timeout
	if len(f.Prices) > 0 {
		p.Prices = f.Prices
	}
	return p, nil
}

// pricingSetup is the pricing form's current state, for the setup page.
type pricingSetup struct {
	pricingForm
	Detected []detected  `json:"detected"`
	Layers   []layerView `json:"layers"`
	Warnings []string    `json:"warnings,omitempty"`
}

func buildPricingSetup(p prefs) *pricingSetup {
	view := buildPricingView()
	out := &pricingSetup{
		pricingForm: pricingForm{
			TTL: cmpOr(p.Pricing.TTL, "24h"), File: p.Pricing.File, URL: p.Pricing.URL, HeaderEnv: p.Pricing.HeaderEnv,
			Translate: p.Pricing.Translate, Timeout: p.Pricing.Timeout, Prices: p.Pricing.Prices,
		},
		Detected: detectedAgents(), Layers: view.Layers, Warnings: view.Warnings,
	}
	if out.Prices == nil {
		out.Prices = map[string]cost.PriceItem{}
	}
	return out
}

// pricingTest is what trying the form's sources found.
type pricingTest struct {
	Models []pricingTestModel `json:"models"`
	Error  string             `json:"error,omitempty"`
}

type pricingTestModel struct {
	ID     string  `json:"id"`
	Source string  `json:"source"`
	In     float64 `json:"in"`
	Out    float64 `json:"out"`
}

// testPricing runs the sources of form once, against a scratch cache, and saves nothing.
func testPricing(ctx context.Context, form pricingPrefs) pricingTest {
	cfg, _ := form.config()
	scratch, err := os.MkdirTemp("", "gascoigne-pricing-")
	if err != nil {
		return pricingTest{Error: err.Error()}
	}
	defer os.RemoveAll(scratch)
	cfg.StateDir = scratch
	ctx, cancel := context.WithTimeout(ctx, 3*cfg.Timeout+30*time.Second)
	defer cancel()
	out := pricingTest{Models: []pricingTestModel{}}
	if err := pricing.Refresh(ctx, cfg, true); err != nil {
		out.Error = err.Error()
	}
	for _, o := range pricing.Load(cfg).Overrides {
		out.Models = append(out.Models, pricingTestModel{ID: o.Model.ID, Source: o.Source, In: o.Model.Input, Out: o.Model.Output})
	}
	return out
}
