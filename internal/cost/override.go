package cost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
)

// SourceBuiltIn is the source of a price from Models.
const SourceBuiltIn = "built-in"

// Override is a price the user supplied, and where it came from ("static",
// "json", "api", "script"): it beats the built-in price of the same id.
type Override struct {
	Model  Model
	Source string
}

var (
	overridesMu sync.RWMutex
	overrides   []Override

	loaderMu sync.Mutex
	loader   func()
)

// SetLoader registers a function that fills the user's prices and billing mode (SetOverrides,
// SetBilling) the first time either is needed, so commands that never price pay nothing. A
// process that sets them itself first never runs it.
func SetLoader(load func()) {
	loaderMu.Lock()
	loader = load
	loaderMu.Unlock()
}

// ensure runs the loader once. It must not be called with a lock of this package held.
func ensure() {
	loaderMu.Lock()
	load := loader
	loader = nil
	loaderMu.Unlock()
	if load != nil {
		load()
	}
}

// SetOverrides replaces the user's prices. The process holds one table: an
// embedder and the CLI both set it once at start.
func SetOverrides(list []Override) {
	dropLoader()
	list = slices.Clone(list)
	for i := range list {
		list[i].Model.ID = strings.ToLower(strings.TrimSpace(list[i].Model.ID))
	}
	overridesMu.Lock()
	overrides = list
	overridesMu.Unlock()
}

func dropLoader() {
	loaderMu.Lock()
	loader = nil
	loaderMu.Unlock()
}

// Overrides returns the user's prices in force.
func Overrides() []Override {
	ensure()
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	return slices.Clone(overrides)
}

// LookupSource is Lookup, also saying where the price comes from. The longest id
// prefixing the model wins; at equal length the user's price beats the built-in.
func LookupSource(id string) (Model, string) {
	ensure()
	id = strings.ToLower(strings.TrimSpace(id))
	var best Model
	source := SourceBuiltIn
	for _, m := range Models {
		if strings.HasPrefix(id, m.ID) && len(m.ID) > len(best.ID) {
			best = m
		}
	}
	overridesMu.RLock()
	defer overridesMu.RUnlock()
	for _, o := range overrides {
		if o.Model.ID != "" && strings.HasPrefix(id, o.Model.ID) && len(o.Model.ID) >= len(best.ID) {
			best, source = o.Model, o.Source
		}
	}
	return best, source
}

// PriceSheetVersion is the version of the JSON price sheet this build reads.
const PriceSheetVersion = 1

// priceSheet is the JSON the user's file, API and script hand over, in dollars
// per million tokens:
//
//	{"version": 1, "models": {"<model id>": {"in": 3, "out": 15,
//	  "cache_read": 0.3, "cache_write": 3.75, "tokenizer": "claude"}}}
//
// in and out are required. cache_write and cache_read default to in (no caching
// discount assumed), tokenizer to o200k (see Tokenizers).
type priceSheet struct {
	Version int                  `json:"version"`
	Models  map[string]PriceItem `json:"models"`
}

// PriceItem is one model in a price sheet (and in the static prices of the settings).
type PriceItem struct {
	In         *float64 `json:"in"`
	Out        *float64 `json:"out"`
	CacheRead  *float64 `json:"cache_read,omitempty"`
	CacheWrite *float64 `json:"cache_write,omitempty"`
	Tokenizer  string   `json:"tokenizer,omitempty"`
}

// ParsePrices reads a price sheet. source names it in errors and on the
// resulting overrides. The models come back sorted by id.
func ParsePrices(data []byte, source string) ([]Override, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var sheet priceSheet
	if err := dec.Decode(&sheet); err != nil {
		return nil, fmt.Errorf("%s prices: %w", source, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s prices: trailing data after the price sheet", source)
	}
	if sheet.Version > PriceSheetVersion {
		return nil, fmt.Errorf("%s prices: version %d is newer than this gascoigne reads (%d)", source, sheet.Version, PriceSheetVersion)
	}
	if len(sheet.Models) == 0 {
		return nil, fmt.Errorf(`%s prices: no "models"`, source)
	}
	var out []Override
	for _, id := range slices.Sorted(maps.Keys(sheet.Models)) {
		model, err := sheet.Models[id].model(id)
		if err != nil {
			return nil, fmt.Errorf("%s prices: %w", source, err)
		}
		out = append(out, Override{Model: model, Source: source})
	}
	return out, nil
}

func (item PriceItem) model(id string) (Model, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return Model{}, errors.New("a model has no id")
	}
	if item.In == nil || item.Out == nil {
		return Model{}, fmt.Errorf("%s: both in and out are required", id)
	}
	tokenizer := item.Tokenizer
	if tokenizer == "" {
		tokenizer = "o200k"
	}
	if _, ok := Tokenizers[tokenizer]; !ok {
		return Model{}, fmt.Errorf("%s: unknown tokenizer %q", id, tokenizer)
	}
	model := Model{ID: id, Tokenizer: tokenizer, Input: *item.In, Output: *item.Out, CacheWrite: *item.In, CacheRead: *item.In}
	if item.CacheWrite != nil {
		model.CacheWrite = *item.CacheWrite
	}
	if item.CacheRead != nil {
		model.CacheRead = *item.CacheRead
	}
	for name, price := range map[string]float64{"in": model.Input, "out": model.Output, "cache_write": model.CacheWrite, "cache_read": model.CacheRead} {
		if price < 0 {
			return Model{}, fmt.Errorf("%s: %s must not be negative", id, name)
		}
	}
	return model, nil
}

// Sheet renders items as a price sheet that ParsePrices reads back.
func Sheet(items map[string]PriceItem) ([]byte, error) {
	return json.Marshal(priceSheet{Version: PriceSheetVersion, Models: items})
}

// Billing modes: dollars are shown only for pay-per-token API use. Auto asks the detector, per
// agent (SetDetector); api and subscription are the user's word.
const (
	BillingAuto         = "auto"
	BillingAPI          = "api"
	BillingSubscription = "subscription"
)

// Detection is how an agent is billed and what says so.
type Detection struct {
	// Mode is BillingAPI or BillingSubscription.
	Mode string
	// Why is a short phrase for the user: "ANTHROPIC_API_KEY is set", "signed in with a Claude account".
	Why string
}

var (
	billingMu sync.RWMutex
	billing   = BillingAuto
	detector  func(agent string) Detection
)

// SetBilling forces a billing mode for every agent; anything but api or subscription means
// auto: ask the detector.
func SetBilling(mode string) {
	dropLoader()
	if mode != BillingAPI && mode != BillingSubscription {
		mode = BillingAuto
	}
	billingMu.Lock()
	billing = mode
	billingMu.Unlock()
}

// SetDetector registers how auto finds out whether an agent is billed per token. With none,
// every agent is taken to be on the API.
func SetDetector(detect func(agent string) Detection) {
	billingMu.Lock()
	detector = detect
	billingMu.Unlock()
}

// Billing is the mode set by the user: auto, api or subscription.
func Billing() string {
	ensure()
	billingMu.RLock()
	defer billingMu.RUnlock()
	return billing
}

// BillingOf says how an agent is billed: the forced mode, else what the detector finds, else
// the API.
func BillingOf(agent string) Detection {
	mode := Billing()
	billingMu.RLock()
	detect := detector
	billingMu.RUnlock()
	switch {
	case mode != BillingAuto:
		return Detection{Mode: mode, Why: "set by you"}
	case detect != nil:
		if found := detect(agent); found.Mode == BillingAPI || found.Mode == BillingSubscription {
			return found
		}
	}
	return Detection{Mode: BillingAPI, Why: "nothing says otherwise"}
}
