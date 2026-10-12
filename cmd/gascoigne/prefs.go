package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	tmp := prefsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, prefsFile())
}
