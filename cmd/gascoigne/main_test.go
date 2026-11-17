package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A bug: TestSuggest and the remote tests read the user config through available(),
// which refreshed the real ~/.config/gascoigne/config.yaml when its template was stale.
// A test that forgets sandbox(t) must still never reach real user state.
func TestMainKeepsTestsOffRealHome(t *testing.T) {
	for _, name := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME"} {
		value := os.Getenv(name)
		if !strings.HasPrefix(value, os.TempDir()) {
			t.Errorf("%s = %q, want a path under %s", name, value, os.TempDir())
		}
	}
}

// TestMain moves HOME and the XDG dirs into a temp dir for every test in the package,
// so a test that skips sandbox(t) cannot read or rewrite the developer's own config.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "gascoigne-test-home-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "test home:", err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
