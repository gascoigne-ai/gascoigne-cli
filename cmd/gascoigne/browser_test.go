package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBrowserCmd(t *testing.T) {
	const url = "http://127.0.0.1:7878"
	for goos, want := range map[string][]string{
		"darwin":  {"open", url},
		"windows": {"rundll32", "url.dll,FileProtocolHandler", url},
		"linux":   {"xdg-open", url},
		"freebsd": {"xdg-open", url},
	} {
		t.Run(goos, func(t *testing.T) {
			if got := browserCmd(goos, url); !slices.Equal(got, want) {
				t.Errorf("browserCmd(%q) = %q, want %q", goos, got, want)
			}
		})
	}
}

func TestSessionEnv(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"wayland-1", "wayland-1.lock", "bus"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	got := sessionEnv("linux", dir)
	for _, want := range []string{"WAYLAND_DISPLAY=wayland-1", "XDG_RUNTIME_DIR=" + dir, "DBUS_SESSION_BUS_ADDRESS=unix:path=" + filepath.Join(dir, "bus")} {
		if !slices.Contains(got, want) {
			t.Errorf("sessionEnv lacks %s: %q", want, got)
		}
	}
	if sessionEnv("darwin", dir) != nil {
		t.Error("only Linux needs a session found")
	}
}
