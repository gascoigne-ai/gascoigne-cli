// Package pricing collects the prices the user supplies, from up to four places, and keeps the
// remote ones fresh without ever blocking a run:
//
//	static   the settings (Config.Static)
//	json     a price sheet file (Config.File)
//	api      a URL returning a price sheet (Config.URL), cached for the TTL
//	script   a user command printing a price sheet (Config.Translate), cached for the TTL
//
// The command may also translate: with a file or URL set, the file or response goes to its
// stdin and it prints the gascoigne price sheet (see cost.ParsePrices). On a model both give,
// static beats json beats api beats script, and all beat the built-in price (package cost).
package pricing

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// Defaults and limits.
const (
	DefaultTTL     = 24 * time.Hour
	MinTTL         = time.Hour
	DefaultTimeout = 10 * time.Second
	// FailureBackoff is how long a failed remote layer waits before its next automatic try.
	FailureBackoff = time.Hour
	// MaxBytes caps a response and a script's output.
	MaxBytes = 1 << 20

	cacheFile = "prices-cache.json"
	lockFile  = "prices.lock"
)

// Layer names, also the Source of the prices they give.
const (
	LayerStatic = "static"
	LayerJSON   = "json"
	LayerAPI    = "api"
	LayerScript = "script"
)

// Config says where prices come from. The zero value gives none.
type Config struct {
	Static map[string]cost.PriceItem
	// File is a price sheet; with Translate it is the translator's stdin.
	File string
	// URL returns a price sheet (or, with Translate, the translator's stdin).
	URL string
	// HeaderEnv names an environment variable holding one "Name: value" request header, so a
	// token never sits in the settings.
	HeaderEnv string
	// Translate is a command line (see SplitArgs), run without a shell.
	Translate string
	// TTL is how long fetched and translated prices stay fresh; 0 means DefaultTTL, unless Manual.
	TTL time.Duration
	// Manual never refreshes by itself: only Refresh with force does.
	Manual bool
	// Timeout bounds one fetch or script run; 0 means DefaultTimeout.
	Timeout time.Duration
	// StateDir holds the cache and the lock.
	StateDir string

	// Client and Now are replaced by tests.
	Client *http.Client
	Now    func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Config) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultTTL
}

func (c Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// ParseTTL reads the interval setting: a Go duration, "" for the default, "0" or "manual" for
// no automatic runs. Less than MinTTL is raised to it, with a warning.
func ParseTTL(text string) (ttl time.Duration, manual bool, warning string, err error) {
	switch strings.TrimSpace(text) {
	case "":
		return DefaultTTL, false, "", nil
	case "0", "manual":
		return 0, true, "", nil
	}
	ttl, err = time.ParseDuration(strings.TrimSpace(text))
	if err != nil {
		return 0, false, "", fmt.Errorf("prices ttl %q: want a duration such as 12h, or manual", text)
	}
	if ttl < 0 {
		return 0, false, "", fmt.Errorf("prices ttl %q must not be negative", text)
	}
	if ttl == 0 {
		return 0, true, "", nil
	}
	if ttl < MinTTL {
		return MinTTL, false, fmt.Sprintf("prices ttl %s is below %s: using %s", text, MinTTL, MinTTL), nil
	}
	return ttl, false, "", nil
}

// Layer is the state of one source, for display.
type Layer struct {
	Name    string
	Models  int
	Fetched time.Time // when remote prices were last good; zero for static and a plain file
	Next    time.Time // when an automatic refresh is due; zero if none
	Error   string    // the last failure, cleared by the next good run
}

// Result is what Load found.
type Result struct {
	Overrides []cost.Override
	Layers    []Layer
	// Stale says a remote layer is due for Refresh.
	Stale bool
}

// remote reports which cached layers the config uses.
func (c Config) remote() []string {
	var names []string
	if c.File != "" && c.Translate != "" {
		names = append(names, LayerJSON)
	}
	if c.URL != "" {
		names = append(names, LayerAPI)
	}
	if c.Translate != "" && c.File == "" && c.URL == "" {
		names = append(names, LayerScript)
	}
	return names
}
