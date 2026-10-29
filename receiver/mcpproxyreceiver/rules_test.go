package mcpproxyreceiver

import (
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResolve(t *testing.T) {
	rules := []Rule{
		{"tool_00", ActionShrink},    // ends the search: overrides the broader skip below
		{"tool_0?", ActionSkip},      // glob on the tool name
		{"git/delete_*", ActionLazy}, // with a slash: server name and tool
		{"search_*", ActionLazy},
	}
	tests := []struct {
		server, tool, want string
	}{
		{"git#aaaa1111", "tool_00", ActionShrink},
		{"git#aaaa1111", "tool_07", ActionSkip},
		{"git#aaaa1111", "tool_10", ""},
		{"git#aaaa1111", "delete_branch", ActionLazy},
		{"other#bbbb2222", "delete_branch", ""}, // another server
		{"other", "search_code", ActionLazy},
	}
	for _, tt := range tests {
		if got := Resolve(rules, tt.server, tt.tool); got != tt.want {
			t.Errorf("Resolve(%s, %s) = %q, want %q", tt.server, tt.tool, got, tt.want)
		}
	}
	if Resolve(nil, "git", "x") != "" {
		t.Error("no rules matched something")
	}
}

func TestRuleValidate(t *testing.T) {
	for name, tt := range map[string]struct {
		rule Rule
		ok   bool
	}{
		"skip":            {Rule{"get_*", ActionSkip}, true},
		"server and tool": {Rule{"git/get_*", ActionLazy}, true},
		"unknown action":  {Rule{"get_*", "drop"}, false},
		"no action":       {Rule{"get_*", ""}, false},
		"no match":        {Rule{"", ActionSkip}, false},
		"bad glob":        {Rule{"get_[", ActionSkip}, false},
	} {
		if err := tt.rule.Validate(); (err == nil) != tt.ok {
			t.Errorf("%s: %v", name, err)
		}
	}
	cfg := &Config{Retrieve: "r", Rules: []Rule{{"x", "nope"}}}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "action") {
		t.Errorf("config with a bad rule: %v", err)
	}
}

func TestSavedRulesFollowConfigRules(t *testing.T) {
	r := &Receiver{server: "git#aaaa1111", cfg: &Config{Rules: []Rule{{"a", ActionSkip}}}}
	ctx := WithRules(t.Context(), map[string][]Rule{"git#aaaa1111": {{"b", ActionLazy}}, "other#1": {{"c", ActionLazy}}})
	got := r.rulesFor(ctx)
	if len(got) != 2 || got[0].Match != "a" || got[1].Match != "b" {
		t.Fatalf("rules %v: want the config's, then this server's saved ones", got)
	}
}

// A result rule never undoes a lazy choice: listing and results are separate.
func TestResultRulesKeepLazyChoices(t *testing.T) {
	for _, action := range []string{ActionShrink, ActionSkip} {
		t.Run(action, func(t *testing.T) {
			cfg := &Config{Retrieve: "gascoigne_retrieve", Rules: []Rule{{"tool_*", action}}}
			got := names(startProxy(t, realServer(10), cfg, map[string][]string{"fixture-server": lazyNames(5, 10)}).tools(t))
			if slices.Contains(got, "tool_05") || !slices.Contains(got, "tool_04") || !slices.Contains(got, "search_tools") {
				t.Errorf("tool list %v: tool_05 must stay on request, tool_04 listed", got)
			}
		})
	}
}

func TestRulesInProxy(t *testing.T) {
	cfg := &Config{Retrieve: "gascoigne_retrieve", Rules: []Rule{{"tool_00", ActionSkip}, {"tool_0[5-9]", ActionLazy}}}
	p := startProxy(t, realServer(10), cfg, nil)
	text := func(name string) string {
		res, err := p.cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"owner": "o", "repo": "r", "query": "q"}})
		if err != nil {
			t.Fatal(err)
		}
		return res.Content[0].(*mcp.TextContent).Text
	}
	if got := text("tool_00"); got != "tool_00 o/r q" {
		t.Errorf("skipped tool's result was changed: %q", got)
	}
	if got := text("tool_01"); !strings.HasPrefix(got, "short:tool_01") {
		t.Errorf("tool with no rule was not shrunk: %q", got)
	}
	names := names(p.tools(t))
	for _, want := range []string{"tool_00", "tool_04", "search_tools", "call_tool"} {
		if !slices.Contains(names, want) {
			t.Errorf("tool list %v lacks %s", names, want)
		}
	}
	for _, gone := range []string{"tool_05", "tool_09"} {
		if slices.Contains(names, gone) {
			t.Errorf("tool list %v still lists lazy %s", names, gone)
		}
	}
	res, err := p.cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "tool_07", "arguments": map[string]any{"query": "q"}}})
	if err != nil || !strings.HasPrefix(res.Content[0].(*mcp.TextContent).Text, "short:tool_07") {
		t.Errorf("a lazy tool is reachable through call_tool: %v %+v", err, res)
	}
}
