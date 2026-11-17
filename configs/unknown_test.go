package configs

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestOverlayKeepsUserVersion(t *testing.T) {
	tests := []struct{ name, user, want string }{
		{"user wins", "version: 2\n", "version: 2"},
		{"no key keeps the default", "router: {}\n", "version: 1"},
		{"null keeps the default", "version:\n", "version: 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Overlay([]byte("version: 1\nrouter: {}\n"), []byte(tt.user))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), tt.want+"\n") || strings.Count(string(out), "version:") != 1 {
				t.Errorf("overlay = %q, want %q once", out, tt.want)
			}
		})
	}
}

func TestHasSettingsIgnoresVersion(t *testing.T) {
	tests := []struct {
		name, data string
		want       bool
	}{
		{"version only", "version: 1\n", false},
		{"version and comments", "## doc\nversion: 1\n# x: 1\n", false},
		{"version and a setting", "version: 1\nrouter: { default: [a] }\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc yaml.Node
			if err := yaml.Unmarshal([]byte(tt.data), &doc); err != nil {
				t.Fatal(err)
			}
			if got := HasSettings(&doc); got != tt.want {
				t.Errorf("HasSettings = %v, want %v", got, tt.want)
			}
		})
	}
}
