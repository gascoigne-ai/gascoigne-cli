package version

import (
	"runtime/debug"
	"testing"
)

func TestCommit(t *testing.T) {
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "aac1e96f0d4b2c1e8a7f6e5d4c3b2a1908172635"}
	tests := []struct {
		name     string
		settings []debug.BuildSetting
		want     string
	}{
		{"clean", []debug.BuildSetting{rev, {Key: "vcs.modified", Value: "false"}}, "aac1e96"},
		{"dirty", []debug.BuildSetting{rev, {Key: "vcs.modified", Value: "true"}}, "aac1e96-dirty"},
		{"no vcs info", nil, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commit(tt.settings); got != tt.want {
				t.Errorf("commit = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"0.3.0", "0.2.2", true},
		{"0.2.2", "0.2.2-dev", true}, // the release is after its dev builds
		{"0.2.2-dev", "0.2.2", false},
		{"0.2.2", "0.2.2", false},
		{"0.2.10", "0.2.9", true}, // numbers, not text
		{"1.0.0", "0.99.99", true},
		{"0.3.0", "0.3.0-dev", true},
		{"0.2.1", "0.3.0", false},
		{"", "0.2.1", false},
		{"latest", "0.2.1", false},
		{"0.3", "0.2.1", false},
		{"0.3.0", "dev", false},
		{"0.3.x", "0.2.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			if got := Newer(tt.a, tt.b); got != tt.want {
				t.Errorf("Newer(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
