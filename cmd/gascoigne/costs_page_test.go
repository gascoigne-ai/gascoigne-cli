package main

import (
	"strings"
	"testing"
)

// The money pages must say what their dollars are: API-equivalent estimates,
// not spend, with the assumed share and the Claude Code scope named.
func TestMoneyPagesSayWhatTheyShow(t *testing.T) {
	for _, c := range []struct {
		page string
		html []byte
		want []string
	}{
		{"costs", costsHTML, []string{"API-equivalent", "recorded no model", "factor not measured", "Not subtracted"}},
		{"context", contextHTML, []string{"Claude Code only", "main conversation", "Likely cause", "Total with subagents"}},
	} {
		for _, w := range c.want {
			if !strings.Contains(string(c.html), w) {
				t.Errorf("%s page lacks %q", c.page, w)
			}
		}
	}
}

// Costs keeps to the estimate from saved tokens: no family table that repeats
// the model table, no proxy tables (the Context page shows them), and the same
// periods as Context so the two pages compare.
func TestCostsPageLayout(t *testing.T) {
	page := string(costsHTML)
	for _, gone := range []string{"By model family", "Measured by the proxy", "Rereads assumed", "Last 4 weeks"} {
		if strings.Contains(page, gone) {
			t.Errorf("costs page still has %q", gone)
		}
	}
	for _, p := range []string{`value="24h"`, `value="7d"`, `value="14d" selected`, `value="30d"`, `value="90d"`} {
		if !strings.Contains(page, p) || !strings.Contains(string(contextHTML), strings.ReplaceAll(p, " selected", "")) {
			t.Errorf("period %s missing on a page", p)
		}
	}
	if !strings.Contains(string(contextHTML), "Requests through the proxy") {
		t.Error("context page lacks the proxy requests table")
	}
}

// replaceChildren writes a null as the text "null": the Costs page showed
// "null null" when two optional notes were empty. The pages filter them out
// and draw their data as charts, not only tables.
func TestPagesDrawChartsAndSkipEmptyNotes(t *testing.T) {
	costs, context := string(costsHTML), string(contextHTML)
	for _, want := range []string{".filter(Boolean)", "G.hbars(", "Where the worth comes from"} {
		if !strings.Contains(costs, want) {
			t.Errorf("costs page lacks %q", want)
		}
	}
	for _, want := range []string{"G.bars(", "Cost per day", "G.area(", `class: "click"`, "Share of cost"} {
		if !strings.Contains(context, want) {
			t.Errorf("context page lacks %q", want)
		}
	}
}

// pages.css gives .wrap to the page container (a bottom padding of 40px), so a
// table cell must not reuse the name: the cache-rewrite causes were pushed to
// the top of rows 60px tall.
func TestContextCellsDoNotReusePageClasses(t *testing.T) {
	if strings.Contains(string(contextHTML), `class: "wrap"`) || strings.Contains(string(contextHTML), "td.wrap") {
		t.Error("a context table cell uses the page container's .wrap class")
	}
}

// The Costs page shows pricing and sends settings nowhere: the dashboard has no login, so the
// settings are changed on the token-protected setup page. The 30-second reload must not redraw it.
func TestCostsPagePricing(t *testing.T) {
	page := string(costsHTML)
	for _, want := range []string{`"/api/pricing"`, `"/api/pricing/refresh"`, `"/api/setup/link"`, "Price from", "Dollars are hidden", "Change pricing in setup", "flat plan", `c.billing === "subscription"`, `"mixed"`} {
		if !strings.Contains(page, want) {
			t.Errorf("costs page lacks %q", want)
		}
	}
	for _, gone := range []string{`"/api/pricing", {`, "readPrices", "Save pricing", `translate:`, `header_env`} {
		if strings.Contains(page, gone) {
			t.Errorf("the page must not edit settings (%q): the dashboard has no login", gone)
		}
	}
	loadFn := page[strings.Index(page, "async function load()"):strings.Index(page, "document.getElementById(\"since\").onchange")]
	if strings.Contains(loadFn, "drawPricing") || strings.Contains(loadFn, "loadPricing") {
		t.Error("the periodic reload redraws the pricing box")
	}
	if strings.Contains(page, "onclick:") {
		t.Error("el sets attributes only: handlers need addEventListener")
	}
}

// Setup carries the interval and the static prices, and shows the terminal-only sources read-only:
// it has no call that tests or sets a file, URL or command.
func TestSetupPagePricingForm(t *testing.T) {
	page := string(setupHTML)
	for _, want := range []string{"pricingPanel", "readPricing", "Refresh every", `costs: "Costs"`, "gascoigne pricing set -file", "gascoigne pricing test", "this page has no login"} {
		if !strings.Contains(page, want) {
			t.Errorf("setup page lacks %q", want)
		}
	}
	for _, gone := range []string{"pricing-test", "p-translate", "p-url", "p-file", "p-header", "p-timeout", "Test these sources"} {
		if strings.Contains(page, gone) {
			t.Errorf("the setup page must not set or test sources (%q): the setup token is minted without a login", gone)
		}
	}
	if strings.Contains(page, "onclick:") {
		t.Error("el sets attributes only: handlers are properties")
	}
}
