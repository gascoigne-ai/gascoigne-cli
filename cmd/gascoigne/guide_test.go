package main

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestGuideBlockRoundTrip(t *testing.T) {
	sandbox(t)
	write(t, "AGENTS.md", "# Project\n\nRun make check.\n")
	for range 2 { // a second install replaces the block
		if _, err := addGuide("AGENTS.md", true); err != nil {
			t.Fatal(err)
		}
	}
	got := read(t, "AGENTS.md")
	if strings.Count(got, guideStart) != 1 || !strings.HasPrefix(got, "# Project\n\nRun make check.\n\n"+guideStart) {
		t.Fatalf("AGENTS.md:\n%s", got)
	}
	if _, err := removeGuide("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "AGENTS.md"); got != "# Project\n\nRun make check.\n" {
		t.Errorf("after remove: %q", got)
	}
}

func TestGuideOnlyFileDeleted(t *testing.T) {
	sandbox(t)
	if _, err := addGuide("AGENTS.md", true); err != nil {
		t.Fatal(err)
	}
	if _, err := removeGuide("AGENTS.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("AGENTS.md"); !os.IsNotExist(err) {
		t.Error("file gascoigne created is still there")
	}
}

func TestInstallWritesGuides(t *testing.T) {
	sandbox(t)
	for _, name := range []string{"codex", "cursor", "pi"} {
		if _, err := agents[name].add("gascoigne", false); err != nil {
			t.Fatal(err)
		}
	}
	if got := read(t, "AGENTS.md"); strings.Count(got, guideStart) != 1 {
		t.Errorf("AGENTS.md shared by codex and pi:\n%s", got)
	}
	if got := read(t, cursorRuleFile); !strings.Contains(got, "alwaysApply: true") {
		t.Errorf("cursor rule:\n%s", got)
	}
	for _, name := range []string{"codex", "cursor", "pi"} {
		if _, err := agents[name].remove(); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"AGENTS.md", cursorRuleFile} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Errorf("%s left after unwrap", f)
		}
	}
}

func TestGuidePromisesOnlyWhatTheAgentHas(t *testing.T) {
	tests := []struct {
		name        string
		mcp         bool
		hasMCPTools bool
	}{
		{"claude, codex, cursor, opencode", true, true},
		{"pi: no MCP", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := guide(tt.mcp)
			if got := strings.Contains(g, "call retrieve"); got != tt.hasMCPTools {
				t.Errorf("retrieve tool present = %v, want %v", got, tt.hasMCPTools)
			}
			if !strings.Contains(g, "[raw:<id>]") || !strings.Contains(g, "gascoigne raw <id>") {
				t.Error("the hint and the CLI form to read the original must stay for every agent")
			}
		})
	}
}

func TestInstallGuideBlocksPerAgent(t *testing.T) {
	sandbox(t)
	if _, err := agents["codex"].add("gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "AGENTS.md"); !strings.Contains(got, "call retrieve") {
		t.Errorf("codex has the MCP tools:\n%s", got)
	}
	if _, err := agents["pi"].add("gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "AGENTS.md"); strings.Contains(got, "call retrieve") {
		t.Errorf("pi has no MCP tools:\n%s", got)
	}
}

func TestGuideLevels(t *testing.T) {
	sandbox(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	tests := []struct {
		level     string
		wantEmpty bool
	}{
		{"short", false},
		{"off", true},
		{"full", false}, // the removed level, still set in older prefs and scripts: short
		{"typo", false}, // unknown: short, never silently off
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Setenv("GASCOIGNE_GUIDE", tt.level)
			g := guide(true)
			if (g == "") != tt.wantEmpty {
				t.Fatalf("empty = %v:\n%s", g == "", g)
			}
			if got := strings.Contains(g, "[raw:<id>]"); got == tt.wantEmpty {
				t.Errorf("hint explanation = %v", got)
			}
			if got := strings.Contains(g, "Keep your own text brief and readable"); got == tt.wantEmpty {
				t.Errorf("brevity advice = %v", got)
			}
			if strings.Contains(g, "Read and search with shell commands") {
				t.Error("the removed full level's tips are back")
			}
		})
	}
	t.Run("default is short", func(t *testing.T) {
		t.Setenv("GASCOIGNE_GUIDE", "")
		if got := guideLevel(); got != "short" {
			t.Errorf("default level = %q", got)
		}
	})
}

func TestGuideLevelFromPrefsAndEnv(t *testing.T) {
	sandbox(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if err := saveGuideLevel("short"); err != nil {
		t.Fatal(err)
	}
	if got := guideLevel(); got != "short" {
		t.Errorf("saved level = %q", got)
	}
	t.Setenv("GASCOIGNE_GUIDE", "off")
	if got := guideLevel(); got != "off" {
		t.Errorf("env must beat the saved level, got %q", got)
	}
	for _, bad := range []string{"loud", "full"} {
		if err := saveGuideLevel(bad); err == nil {
			t.Errorf("level %q must be refused", bad)
		}
	}
}

func TestGuideOffInstallsNoGuide(t *testing.T) {
	sandbox(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, name := range []string{"codex", "cursor"} {
		if _, err := agents[name].add("gascoigne", false); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(read(t, "AGENTS.md"), guideStart) || !strings.Contains(read(t, cursorRuleFile), "alwaysApply") {
		t.Fatal("the default level writes the guide")
	}
	t.Setenv("GASCOIGNE_GUIDE", "off")
	for _, name := range []string{"codex", "cursor"} {
		if _, err := agents[name].add("gascoigne", false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(cursorRuleFile); !os.IsNotExist(err) {
		t.Error("cursor rule stays after level off")
	}
	if _, err := os.Stat("AGENTS.md"); !os.IsNotExist(err) {
		t.Errorf("AGENTS.md held only the guide, it should be gone: %v", err)
	}
}

func TestSessionStartGuideOffAddsNothing(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_GUIDE", "off")
	h := withGuide(func(context.Context, *common, hookInput, string) (any, error) { return nil, nil })
	reply, err := h(t.Context(), &common{}, hookInput{HookEventName: "SessionStart"}, "")
	if err != nil || reply != nil {
		t.Fatalf("level off must not add context: %v, %v", reply, err)
	}
}
