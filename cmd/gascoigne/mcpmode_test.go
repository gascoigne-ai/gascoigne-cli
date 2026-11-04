package main

import (
	"slices"
	"strings"
	"testing"
)

func TestMCPModeFromPrefsAndEnv(t *testing.T) {
	sandbox(t)
	if got := mcpMode(); got != "auto" {
		t.Fatalf("default = %q, want auto", got)
	}
	if err := saveMCPMode("manual"); err != nil {
		t.Fatal(err)
	}
	if got := mcpMode(); got != "manual" {
		t.Errorf("saved = %q", got)
	}
	t.Setenv("GASCOIGNE_MCP_MODE", "auto")
	if got := mcpMode(); got != "auto" {
		t.Errorf("env must beat the saved mode, got %q", got)
	}
	t.Setenv("GASCOIGNE_MCP_MODE", "sometimes")
	if got := mcpMode(); got != "manual" {
		t.Errorf("an unknown env value is ignored, got %q", got)
	}
	if err := saveMCPMode("sometimes"); err == nil {
		t.Error("an unknown mode must be refused")
	}
}

func TestManualModeSkipsOnlyToolRouting(t *testing.T) {
	sandbox(t)
	events := map[string]bool{
		"PreToolUse": true, "PostToolUse": true, "BeforeTool": true, "AfterTool": true,
		"SessionStart": false, "UserPromptSubmit": false, "": true, "Stop": true,
	}
	for event, routes := range events {
		if got := routesTools(event); got != routes {
			t.Errorf("routesTools(%q) = %v, want %v", event, got, routes)
		}
	}
	for event := range events {
		if skipHook(event) {
			t.Errorf("auto mode skipped %q", event)
		}
	}
	t.Setenv("GASCOIGNE_MCP_MODE", "manual")
	for event, routes := range events {
		if got := skipHook(event); got != routes {
			t.Errorf("manual mode: skipHook(%q) = %v, want %v", event, got, routes)
		}
	}
}

func TestManualModeInstallsNoDenyRules(t *testing.T) {
	sandbox(t)
	const settings = ".claude/settings.json"
	denies := func() []string {
		root, err := parseJSON([]byte(read(t, settings)))
		if err != nil {
			t.Fatal(err)
		}
		return denyRules.rules(root.(*object))
	}
	install := func(mode string) {
		t.Helper()
		if err := saveMCPMode(mode); err != nil {
			t.Fatal(err)
		}
		if _, err := addClaude("gascoigne", false); err != nil {
			t.Fatal(err)
		}
	}
	write(t, settings, `{"permissions": {"deny": ["Bash(rm:*)"]}}`)

	install("auto")
	if got := denies(); !slices.Equal(got, []string{"Bash(rm:*)", "Grep", "Glob", "WebFetch"}) {
		t.Fatalf("auto: %v", got)
	}
	install("manual")
	if got := denies(); !slices.Equal(got, []string{"Bash(rm:*)"}) {
		t.Fatalf("manual takes gascoigne's denies back and keeps the user's: %v", got)
	}
	install("manual") // idempotent
	if got := denies(); !slices.Equal(got, []string{"Bash(rm:*)"}) {
		t.Fatalf("manual twice: %v", got)
	}
	install("auto")
	if got := denies(); !slices.Equal(got, []string{"Bash(rm:*)", "Grep", "Glob", "WebFetch"}) {
		t.Fatalf("back to auto: %v", got)
	}
}

func TestManualModeRunArgsDenyNothing(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_MCP_MODE", "manual")
	args, err := claudeArgs("gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range args {
		if arg != "" && strings.Contains(arg, "WebFetch") {
			t.Fatalf("wrap in manual mode denies tools: %s", arg)
		}
	}
}

func TestSetupOffersMCPMode(t *testing.T) {
	sandbox(t)
	var found decision
	for _, d := range decisions(prefs{}) {
		if d.ID == "mcp_mode" {
			found = d
		}
	}
	if found.ID == "" || !found.Optional || found.Current != "auto" || found.Default != "auto" || len(found.Options) != 2 {
		t.Fatalf("decision = %+v", found)
	}
	// An optional question with a current value is never pending.
	for _, d := range pendingDecisions(prefs{}) {
		if d.ID == "mcp_mode" {
			t.Fatal("mcp_mode must not nag")
		}
	}
	if _, err := applyAnswers(map[string]string{"mcp_mode": "manual"}); err != nil {
		t.Fatal(err)
	}
	if p := readPrefs(); p.MCPMode != "manual" {
		t.Fatalf("prefs: %+v", p)
	}
	if _, err := applyAnswers(map[string]string{"mcp_mode": "later"}); err == nil {
		t.Error("an unknown mode must be refused")
	}
}

func TestManualModeGuideTellsTheAgentToCallTools(t *testing.T) {
	sandbox(t)
	auto := guide(true)
	t.Setenv("GASCOIGNE_MCP_MODE", "manual")
	manual := guide(true)
	if strings.Contains(manual, "shrinks shell output") || !strings.Contains(manual, "does not shorten") {
		t.Fatalf("manual guide must not claim automatic shrinking:\n%s", manual)
	}
	for _, tool := range []string{"fetch", "compress", "retrieve"} {
		if !strings.Contains(manual, tool) {
			t.Errorf("manual guide never names %s:\n%s", tool, manual)
		}
	}
	if !strings.Contains(auto, "shrinks shell output") {
		t.Fatalf("auto guide changed:\n%s", auto)
	}
	t.Setenv("GASCOIGNE_GUIDE", "off")
	if guide(true) != "" {
		t.Error("guide off stays off in manual mode")
	}
}
