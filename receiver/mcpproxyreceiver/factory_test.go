package mcpproxyreceiver

import (
	"slices"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

func TestFactoryAppliesOptions(t *testing.T) {
	id := mcptool.Identity{Session: "s1", Agent: "claude-code"}
	lazy := map[string][]string{"git#aaaa1111": {"b"}}
	rules := map[string][]Rule{"git#aaaa1111": {{"b", ActionLazy}}}
	f := NewFactory(WithCommand([]string{"git-mcp", "-x"}), WithIdentity(id), WithLazy(lazy), WithRules(rules))
	c, err := f.Create(t.Context(), component.Settings{Pipeline: pipeline{}}, f.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	r := c.(*Receiver)
	if !slices.Equal(r.commandLine(), []string{"git-mcp", "-x"}) || r.identity != id || r.lazy["git#aaaa1111"][0] != "b" || r.rules["git#aaaa1111"][0].Match != "b" {
		t.Fatalf("options not applied: %+v", r)
	}
}

func TestConfigServerWithoutOptions(t *testing.T) {
	r := &Receiver{cfg: &Config{Retrieve: "r", Command: []string{"cfg-server"}}}
	if got := r.commandLine(); !slices.Equal(got, []string{"cfg-server"}) {
		t.Fatalf("command %v, want the configured one", got)
	}
	if got := r.serverIdentity(); !slices.Equal(got, []string{"cfg-server"}) {
		t.Fatalf("identity %v", got)
	}
	WithRemote(Remote{URL: "https://x.test/mcp"})(r)
	if got := r.serverIdentity(); !slices.Equal(got, []string{"https://x.test/mcp"}) {
		t.Fatalf("identity %v, want the remote's url", got)
	}
}
