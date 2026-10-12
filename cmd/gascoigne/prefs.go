package main

import (
	"encoding/json"
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
	// ReleaseCheck (on or off; unset = on) lets the background server read the
	// website's latest.json once a day; SeenRelease is the release whose page
	// was already opened.
	ReleaseCheck string `json:"release_check,omitempty"`
	SeenRelease  string `json:"seen_release,omitempty"`
	// Declined are agents found on PATH that the user chose not to integrate.
	Declined []string `json:"declined,omitempty"`
	// UpdateDeclined is the registrationVersion whose hook update the user
	// declined: asked again with the next one.
	UpdateDeclined int `json:"update_declined,omitempty"`
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
