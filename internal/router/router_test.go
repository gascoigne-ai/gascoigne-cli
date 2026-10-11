package router

import (
	"slices"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestGlob(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"git", "git", true},
		{"git", "gitx", false},
		{"git *", "git log -- a/b", true},
		{"git *", "go test", false},
		{"*.json", "/tmp/a.json", true},
		{"*test*", "go test ./...", true},
		{"a*b*c", "abc", true},
		{"a*b*c", "acb", false},
		{"*", "", true},
	}
	for _, tt := range tests {
		if got := Glob(tt.pattern, tt.s); got != tt.want {
			t.Errorf("Glob(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestRoute(t *testing.T) {
	cfg := Config{
		Routes: []Route{
			{Name: "git", Match: Match{Attrs: map[string]string{"cmd": "git *"}}, Optimizers: []string{"g"}},
			{Match: Match{Kind: payload.JSON, MinTokens: 10}, Optimizers: []string{"j"}},
			{Name: "go", Match: Match{Content: "go-*"}, Optimizers: []string{"t"}},
			{Name: "git", Match: Match{Content: "git-*"}, Optimizers: []string{"gl"}},
		},
		Default: []string{"d"},
	}
	withCmd := func(cmd string) *payload.Payload {
		p := payload.New(payload.CmdOutput, "x")
		p.Set("cmd", cmd)
		return p
	}
	typed := func(typ string) *payload.Payload {
		return &payload.Payload{Blocks: []payload.Block{{Kind: payload.CmdOutput, Text: "x"}, {Kind: payload.CmdOutput, Text: "y", Type: typ}}}
	}
	tests := []struct {
		name      string
		p         *payload.Payload
		wantRoute string
		wantOpts  []string
	}{
		{"attr glob", withCmd("git status"), "git", []string{"g"}},
		{"attr miss", withCmd("ls"), DefaultRoute, []string{"d"}},
		{"kind+size", payload.New(payload.JSON, strings.Repeat("x ", 40)), "route1", []string{"j"}},
		{"kind too small", payload.New(payload.JSON, "{}"), DefaultRoute, []string{"d"}},
		{"kind miss", payload.New(payload.Message, strings.Repeat("x", 40)), DefaultRoute, []string{"d"}},
		{"content", typed("go-test"), "go", []string{"t"}},
		{"content glob", typed("git-log"), "git", []string{"gl"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route, opts := cfg.Route(tt.p)
			if route != tt.wantRoute || !slices.Equal(opts, tt.wantOpts) {
				t.Fatalf("Route = %s %v, want %s %v", route, opts, tt.wantRoute, tt.wantOpts)
			}
		})
	}
}
