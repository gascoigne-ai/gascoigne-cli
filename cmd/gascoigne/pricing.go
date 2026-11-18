package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pricing"
)

// pricingPrefs are the user's own prices and where else to get them (see internal/pricing).
// They live in the prefs, not in a project file: the file, URL and command settings read files,
// reach the network and run programs, so only the user, at their own terminal, sets them.
type pricingPrefs struct {
	// Billing is "auto" (default: found out per agent from how it is signed in), "api" or
	// "subscription": under a flat plan no dollars are shown.
	Billing string `json:"billing,omitempty"`
	// Prices are the static prices, by model id.
	Prices map[string]cost.PriceItem `json:"prices,omitempty"`
	// File, URL and Translate: see pricing.Config.
	File      string `json:"file,omitempty"`
	URL       string `json:"url,omitempty"`
	HeaderEnv string `json:"header_env,omitempty"`
	Translate string `json:"translate,omitempty"`
	// TTL is how often remote prices refresh: a duration, or "manual". Timeout bounds one run.
	TTL     string `json:"ttl,omitempty"`
	Timeout string `json:"timeout,omitempty"`
}

// config turns the prefs into the pricing package's settings. Warnings are for display.
func (p pricingPrefs) config() (cfg pricing.Config, warnings []string) {
	cfg = pricing.Config{
		Static: p.Prices, File: p.File, URL: p.URL, HeaderEnv: p.HeaderEnv, Translate: p.Translate,
		StateDir: stateDir(),
	}
	ttl, manual, warning, err := pricing.ParseTTL(p.TTL)
	if err != nil {
		warnings = append(warnings, err.Error()+": using the default")
		ttl = pricing.DefaultTTL
	}
	if warning != "" {
		warnings = append(warnings, warning)
	}
	cfg.TTL, cfg.Manual = ttl, manual
	if p.Timeout != "" {
		if timeout, err := time.ParseDuration(p.Timeout); err == nil && timeout > 0 {
			cfg.Timeout = timeout
		} else {
			warnings = append(warnings, fmt.Sprintf("prices timeout %q: using the default", p.Timeout))
		}
	}
	return cfg, warnings
}

// installPricing makes the process read the user's prices and billing mode when it first
// prices something.
func installPricing() {
	cost.SetDetector(detectBilling)
	cost.SetLoader(func() { applyPricing(true) })
}

// applyPricing puts the user's prices and billing mode in force. A stale remote source is
// refreshed in a detached process (spawn), so this never waits for the network or a script.
func applyPricing(spawn bool) pricing.Result {
	p := readPrefs().Pricing
	cfg, _ := p.config()
	result := pricing.Load(cfg)
	cost.SetOverrides(result.Overrides)
	cost.SetBilling(p.Billing)
	if spawn && result.Stale {
		spawnPricingRefresh()
	}
	return result
}

// spawnPricingRefresh runs "gascoigne pricing refresh -auto" detached. The lock file makes
// concurrent callers run it once.
func spawnPricingRefresh() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	command := exec.Command(exe, "pricing", "refresh", "-auto")
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	detach(command)
	if command.Start() == nil {
		command.Process.Release()
	}
}

func cmdPricing(ctx context.Context, args []string) error {
	sub := "show"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "show":
		if err := newFlags("pricing").Parse(args); err != nil {
			return err
		}
		return runPricingShow(os.Stdout)
	case "set":
		return runPricingSet(os.Stdout, args)
	case "refresh":
		fset := newFlags("pricing")
		auto := fset.Bool("auto", false, "run only what is due, quietly (what a stale source starts by itself)")
		if err := fset.Parse(args); err != nil {
			return err
		}
		cfg, _ := readPrefs().Pricing.config()
		err := pricing.Refresh(ctx, cfg, !*auto)
		if *auto {
			return nil // the status shows the error
		}
		if err != nil {
			return err
		}
		return runPricingShow(os.Stdout)
	case "test":
		if err := newFlags("pricing").Parse(args); err != nil {
			return err
		}
		return runPricingTest(ctx, os.Stdout)
	}
	return fmt.Errorf("pricing: unknown subcommand %q; want show, set, refresh or test", sub)
}

// runPricingShow prints the billing mode, each source and what it gave.
func runPricingShow(writer io.Writer) error {
	p := readPrefs().Pricing
	cfg, warnings := p.config()
	result := pricing.Load(cfg)
	fmt.Fprintf(writer, "billing: %s\n", cmp.Or(p.Billing, cost.BillingAuto))
	switch p.Billing {
	case cost.BillingSubscription:
		fmt.Fprintln(writer, "  flat plan: dollar figures are hidden (gascoigne pricing set -billing api shows them)")
	case cost.BillingAPI:
		fmt.Fprintln(writer, "  pay per token: dollar figures are shown for every agent")
	default:
		fmt.Fprintln(writer, "  found out per agent from how it is signed in (set -billing api or subscription to decide yourself):")
		for _, d := range detectedAgents() {
			fmt.Fprintf(writer, "    %-8s %-13s %s\n", d.Agent, d.Mode, d.Why)
		}
	}
	ttl := "manual"
	if !cfg.Manual {
		ttl = cfg.TTL.String()
	}
	fmt.Fprintf(writer, "refresh every: %s\n", ttl)
	tw := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "source\tmodels\tlast good\tnext\terror\t")
	fmt.Fprintln(tw, "built-in\t"+strconv.Itoa(len(cost.Models))+"\tlist prices checked "+cost.PricesChecked+"\t\t\t")
	for _, layer := range result.Layers {
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\t%s\t\n", layer.Name, layer.Models, when(layer.Fetched), when(layer.Next), layer.Error)
	}
	tw.Flush()
	for _, o := range result.Overrides {
		fmt.Fprintf(writer, "  %-28s in %-7g out %-7g cache read %-7g cache write %-7g %s  (%s)\n", o.Model.ID,
			o.Model.Input, o.Model.Output, o.Model.CacheRead, o.Model.CacheWrite, o.Model.Tokenizer, o.Source)
	}
	for _, warning := range warnings {
		fmt.Fprintln(writer, "warning:", warning)
	}
	for _, setting := range []struct{ name, value string }{{"file", p.File}, {"url", p.URL}, {"header env", p.HeaderEnv}, {"translate", p.Translate}} {
		if setting.value != "" {
			fmt.Fprintf(writer, "%s: %s\n", setting.name, setting.value)
		}
	}
	return nil
}

func when(moment time.Time) string {
	if moment.IsZero() {
		return "-"
	}
	return moment.Local().Format("2006-01-02 15:04")
}

// runPricingTest runs every remote source once against a scratch cache and prints what they
// give, saving nothing: the way to try a new file, URL or script.
func runPricingTest(ctx context.Context, writer io.Writer) error {
	cfg, _ := readPrefs().Pricing.config()
	scratch, err := os.MkdirTemp("", "gascoigne-pricing-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	cfg.StateDir = scratch
	refreshErr := pricing.Refresh(ctx, cfg, true)
	result := pricing.Load(cfg)
	for _, o := range result.Overrides {
		fmt.Fprintf(writer, "%-28s in %-7g out %-7g cache read %-7g cache write %-7g %s  (%s)\n", o.Model.ID,
			o.Model.Input, o.Model.Output, o.Model.CacheRead, o.Model.CacheWrite, o.Model.Tokenizer, o.Source)
	}
	if len(result.Overrides) == 0 && refreshErr == nil {
		fmt.Fprintln(writer, "no prices: set a file, URL, command or static prices first")
	}
	return refreshErr
}

// billingChoices are the values of the billing setting.
var billingChoices = []string{cost.BillingAuto, cost.BillingAPI, cost.BillingSubscription}

// priceFlag collects -price model=in,out[,cache_read[,cache_write[,tokenizer]]].
type priceFlag map[string]cost.PriceItem

func (f priceFlag) String() string { return "" }

func (f priceFlag) Set(value string) error {
	id, rest, ok := strings.Cut(value, "=")
	id = strings.TrimSpace(id)
	if !ok || id == "" {
		return fmt.Errorf("-price %q: want model=in,out[,cache_read,cache_write[,tokenizer]]", value)
	}
	parts := strings.Split(rest, ",")
	if len(parts) < 2 || len(parts) > 5 {
		return fmt.Errorf("-price %q: want model=in,out[,cache_read,cache_write[,tokenizer]]", value)
	}
	numbers := make([]*float64, 4)
	for i := 0; i < len(parts) && i < 4; i++ {
		n, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
		if err != nil || n < 0 {
			return fmt.Errorf("-price %q: %q is not a price in dollars per million tokens", value, parts[i])
		}
		numbers[i] = &n
	}
	item := cost.PriceItem{In: numbers[0], Out: numbers[1], CacheRead: numbers[2], CacheWrite: numbers[3]}
	if len(parts) == 5 {
		item.Tokenizer = strings.TrimSpace(parts[4])
	}
	f[strings.ToLower(id)] = item
	return nil
}

// validateStatic makes sure the item parses the way a price sheet would.
func validateStatic(prices map[string]cost.PriceItem) error {
	if len(prices) == 0 {
		return nil
	}
	sheet, err := cost.Sheet(prices)
	if err != nil {
		return err
	}
	_, err = cost.ParsePrices(sheet, "static")
	return err
}

// runPricingSet changes the pricing prefs. Nothing is saved unless every flag is valid.
func runPricingSet(writer io.Writer, args []string) error {
	fset := newFlags("pricing")
	flags := registerPricingSetFlags(fset)
	if err := fset.Parse(args); err != nil {
		return err
	}
	prefs := readPrefs()
	fset.Visit(func(setting *flag.Flag) { flags.set[setting.Name] = true })
	if err := flags.apply(writer, &prefs.Pricing); err != nil {
		return err
	}
	if err := prefs.save(); err != nil {
		return err
	}
	fmt.Fprintln(writer, "pricing settings saved; gascoigne pricing shows them, gascoigne pricing test tries the sources")
	return nil
}

// pricingSetFlags are the flags of pricing set; set names the ones given.
type pricingSetFlags struct {
	billing, ttl, file, url, headerEnv, translate, timeout string
	prices                                                 priceFlag
	unset                                                  listFlag
	clearAll                                               bool
	set                                                    map[string]bool
}

func registerPricingSetFlags(fset *flag.FlagSet) *pricingSetFlags {
	flags := &pricingSetFlags{prices: priceFlag{}, set: map[string]bool{}}
	fset.StringVar(&flags.billing, "billing", "", "auto (find out per agent), api (dollars from list prices or yours) or subscription (flat plan: no dollars)")
	fset.StringVar(&flags.ttl, "ttl", "", "how often a URL or command refreshes: a duration (24h, minimum 1h) or manual")
	fset.StringVar(&flags.file, "file", "", "price sheet file (JSON); with -translate its content goes to the command")
	fset.StringVar(&flags.url, "url", "", "URL returning a price sheet, refreshed every ttl")
	fset.StringVar(&flags.headerEnv, "header-env", "", "name of an environment variable holding one 'Name: value' header for the URL request")
	fset.StringVar(&flags.translate, "translate", "", "command that prints a price sheet: it reads the file or URL content on stdin (or runs alone), once per ttl; no shell")
	fset.StringVar(&flags.timeout, "timeout", "", "limit for one fetch or command run (default 10s)")
	fset.Var(flags.prices, "price", "static price: model=in,out[,cache_read,cache_write[,tokenizer]] in dollars per million tokens (repeatable)")
	fset.Var(&flags.unset, "unset-price", "remove a static price by model id (repeatable)")
	fset.BoolVar(&flags.clearAll, "clear", false, "remove every pricing setting")
	return flags
}

// checkPricingURL accepts no address (clears it) or an http(s) one.
func checkPricingURL(url string) error {
	if url != "" && !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("pricing: -url wants an http or https address")
	}
	return nil
}

// checkPricingTranslate accepts no command (clears it) or one that splits.
func checkPricingTranslate(translate string) error {
	if translate == "" {
		return nil
	}
	if _, err := pricing.SplitArgs(translate); err != nil {
		return fmt.Errorf("pricing: -translate: %w", err)
	}
	return nil
}

// checkPricingTimeout accepts no timeout (clears it) or a positive duration.
func checkPricingTimeout(timeout string) error {
	if timeout == "" {
		return nil
	}
	if duration, err := time.ParseDuration(timeout); err != nil || duration <= 0 {
		return fmt.Errorf("pricing: -timeout %q: want a duration such as 10s", timeout)
	}
	return nil
}

// apply validates the settings given and writes them into prefs.
func (flags *pricingSetFlags) apply(writer io.Writer, prefs *pricingPrefs) error {
	if flags.clearAll {
		*prefs = pricingPrefs{}
	}
	if flags.set["billing"] {
		if !slices.Contains(billingChoices, flags.billing) {
			return fmt.Errorf("pricing: -billing wants auto, api or subscription")
		}
		prefs.Billing = flags.billing
	}
	if flags.set["ttl"] {
		ttl, err := flags.checkedTTL(writer)
		if err != nil {
			return err
		}
		prefs.TTL = ttl
	}
	if flags.set["file"] {
		prefs.File = flags.file
	}
	if flags.set["url"] {
		if err := checkPricingURL(flags.url); err != nil {
			return err
		}
		prefs.URL = flags.url
	}
	if flags.set["header-env"] {
		prefs.HeaderEnv = flags.headerEnv
	}
	if flags.set["translate"] {
		if err := checkPricingTranslate(flags.translate); err != nil {
			return err
		}
		prefs.Translate = flags.translate
	}
	if flags.set["timeout"] {
		if err := checkPricingTimeout(flags.timeout); err != nil {
			return err
		}
		prefs.Timeout = flags.timeout
	}
	if len(flags.prices) > 0 || len(flags.unset) > 0 {
		return flags.applyPrices(prefs)
	}
	return nil
}

// checkedTTL returns the ttl to save, warning when it was raised to the minimum.
func (flags *pricingSetFlags) checkedTTL(writer io.Writer) (string, error) {
	duration, manual, warning, err := pricing.ParseTTL(flags.ttl)
	if err != nil {
		return "", err
	}
	if warning != "" {
		fmt.Fprintln(writer, "warning:", warning)
	}
	switch {
	case manual:
		return "manual", nil
	case warning != "":
		return duration.String(), nil // the raised value, not what was typed
	}
	return strings.TrimSpace(flags.ttl), nil
}

// applyPrices merges -price into the saved static prices and drops -unset-price.
func (flags *pricingSetFlags) applyPrices(prefs *pricingPrefs) error {
	merged := map[string]cost.PriceItem{}
	for id, item := range prefs.Prices {
		merged[id] = item
	}
	for id, item := range flags.prices {
		merged[id] = item
	}
	for _, id := range flags.unset {
		delete(merged, strings.ToLower(strings.TrimSpace(id)))
	}
	if len(merged) == 0 {
		merged = nil
	}
	if err := validateStatic(merged); err != nil {
		return fmt.Errorf("pricing: %w", err)
	}
	prefs.Prices = merged
	return nil
}
