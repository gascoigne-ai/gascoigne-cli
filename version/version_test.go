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
