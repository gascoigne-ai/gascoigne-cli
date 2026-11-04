package main

import (
	"encoding/json"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
	"os"
	"path/filepath"
	"time"
)

// stateDir is where gascoigne keeps what it records about its own installs
// and the user's answers (not the ledger, which is data, nor the config).
func stateDir() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(dir, "gascoigne")
}

// prefs are settings that live outside config.yaml, which would need a new
// key in the engine's config: small, per user, written by commands.
type prefs struct {
	// Guide is how much usage text agents get: off, short or full (default).
	Guide string `json:"guide,omitempty"`
	// Allow is which gascoigne MCP tools run without an approval prompt:
	// none (default), local (read-only ones) or all (adds fetch).
	Allow string `json:"allow,omitempty"`
	// MCPMode is how gascoigne reaches the agent: auto (default) routes tool
	// results through hooks, manual leaves it to the agent calling the MCP tools.
	MCPMode string `json:"mcp_mode,omitempty"`
	// Pricing is where dollar figures come from: the billing mode, the user's own prices and the
	// file, URL or command that supply more (see pricing.go).
	Pricing pricingPrefs `json:"pricing,omitzero"`
	// Autosync (on or off) lets agent hooks bring user-scope installs made
	// by an older gascoigne up to date; unset = not decided, nothing is synced.
	Autosync string `json:"autosync,omitempty"`
	// AskedAt is when the setup page was last opened by install or a session
	// start: it comes back with a new session while questions stay open.
	AskedAt time.Time `json:"asked_at,omitzero"`
	// Experimental lists the experimental features the user turned on in
	// setup (config.yaml's experimental list and $GASCOIGNE_EXPERIMENTAL
	// still count).
	Experimental []string `json:"experimental,omitempty"`
	// ProseModel is the prose model chosen in setup; "" leaves it to
	// optimizers.prose.model in the config.
	ProseModel string `json:"prose_model,omitempty"`
	// ReleaseCheck (on or off; unset = on) lets gascoigne read the
	// website's latest.json every 15 minutes; SeenRelease is the release whose page
	// was already opened.
	ReleaseCheck string `json:"release_check,omitempty"`
	SeenRelease  string `json:"seen_release,omitempty"`
	// Window is the auto-compact window (tokens) install -window wrote into
	// claude's settings; unwrap removes the key only while it still holds it.
	Window int `json:"window,omitempty"`
	// SetupVersion is the release whose setup page a session start opened;
	// a new release opens it once more, to finish the update.
	SetupVersion string `json:"setup_version,omitempty"`
	// Declined are agents found on PATH that the user chose not to integrate.
	Declined []string `json:"declined,omitempty"`
	// UpdateDeclined is the registrationVersion whose hook update the user
	// declined: asked again with the next one.
	UpdateDeclined int `json:"update_declined,omitempty"`
	// LazyTools are the MCP tools the user chose to load on request only, by
	// server id (name#hash, see mcpreview.go): mcp-proxy lists the others and
	// reaches these through search_tools and call_tool. Set on the review page.
	LazyTools map[string][]string `json:"lazy_tools,omitempty"`
	// MCPReview (on or off; unset = on) lets a session start open the MCP review
	// page; LastMCPReview is when it last did, at most once a day.
	// LearnedMCP are the review's server ids the user asked to judge now, without
	// waiting for observeDays.
	LearnedMCP []string `json:"learned_mcp,omitempty"`
	// MCPRules are the manual rules set on the review page, by server id; the
	// config's mcp_proxy.rules come first.
	MCPRules      map[string][]mcpproxyreceiver.Rule `json:"mcp_rules,omitempty"`
	MCPReview     string                             `json:"mcp_review,omitempty"`
	LastMCPReview time.Time                          `json:"last_mcp_review,omitzero"`
}

func prefsFile() string { return filepath.Join(stateDir(), "prefs.json") }

// readPrefs returns the saved prefs; a missing or unreadable file means none.
func readPrefs() prefs {
	var p prefs
	if data, err := os.ReadFile(prefsFile()); err == nil {
		json.Unmarshal(data, &p)
	}
	return p
}

// save writes p atomically: hooks read the file while commands write it.
func (p prefs) save() error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	tmp := prefsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, prefsFile())
}
