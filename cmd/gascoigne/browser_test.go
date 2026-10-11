package main

import (
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
