package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

func TestLeftovers(t *testing.T) {
	home := sandbox(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	custom := filepath.Join(home, "elsewhere", "my.db")
	t.Setenv(ledger.EnvPath, custom)
	// Where these live depends on the OS (macOS ignores the XDG variables).
	cfg, _ := os.UserConfigDir()
	cache, _ := os.UserCacheDir()
	write(t, filepath.Join(cfg, "gascoigne/config.yaml"), "x")
	write(t, filepath.Join(cache, "gascoigne/server.log"), "x")
	for _, f := range []string{
		".local/share/gascoigne/ledger.db",
		".local/state/gascoigne/denies.json",
		"elsewhere/my.db",
		"elsewhere/my.db-wal",
		"elsewhere/keep.txt",
	} {
		write(t, filepath.Join(home, f), "x")
	}
	server := filepath.Join(home, ".local/share/gascoigne/ledger.db") // inside a removed dir

	got := leftovers(server)
	want := []string{
		filepath.Join(cfg, "gascoigne"),
		filepath.Join(home, ".local/share/gascoigne"),
		filepath.Join(home, ".local/state/gascoigne"),
		filepath.Join(cache, "gascoigne"),
		custom,
		custom + "-wal",
	}
	if !slices.Equal(got, want) {
		t.Errorf("leftovers:\n got %q\nwant %q", got, want)
	}
}

func TestLeftoversNone(t *testing.T) {
	home := sandbox(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv(ledger.EnvPath, "")
	if got := leftovers(); len(got) != 0 {
		t.Errorf("leftovers in an empty home: %q", got)
	}
}

func TestConfirm(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"YES\n", true},
		{" y ", true},
		{"n\n", false},
		{"\n", false},
		{"", false},
		{"yep\n", false},
	} {
		t.Run(tc.in, func(t *testing.T) {
			var out bytes.Buffer
			if got := confirm(strings.NewReader(tc.in), &out); got != tc.want {
				t.Errorf("confirm(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
