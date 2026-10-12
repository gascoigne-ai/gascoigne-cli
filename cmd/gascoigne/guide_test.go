package main

import (
	"os"
	"strings"
	"testing"
)

func TestGuideBlockRoundTrip(t *testing.T) {
	sandbox(t)
	write(t, "AGENTS.md", "# Project\n\nRun make check.\n")
	for range 2 { // a second install replaces the block
		if _, err := addGuide("AGENTS.md", true, true); err != nil {
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
	if _, err := addGuide("AGENTS.md", true, true); err != nil {
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
		name             string
		rewrites, mcp    bool
		hasRewriteAdvice bool
		hasMCPTools      bool
	}{
		{"claude, cursor, opencode", true, true, true, true},
		{"codex: no rewrites", false, true, false, true},
		{"pi: no MCP", true, false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := guide(tt.rewrites, tt.mcp)
			if got := strings.Contains(g, "Loops, $( )"); got != tt.hasRewriteAdvice {
				t.Errorf("rewrite advice = %v, want %v", got, tt.hasRewriteAdvice)
			}
			for _, tool := range []string{"gascoigne fetch tool", "compress", "the show tool", "call retrieve"} {
				if got := strings.Contains(g, tool); got != tt.hasMCPTools {
					t.Errorf("%q present = %v, want %v", tool, got, tt.hasMCPTools)
				}
			}
			if !strings.Contains(g, "gascoigne show <file>:<Name>") || !strings.Contains(g, "gascoigne raw <id>") {
				t.Error("the CLI forms must stay for every agent")
			}
		})
	}
}

func TestInstallGuideBlocksPerAgent(t *testing.T) {
	sandbox(t)
	if _, err := agents["codex"].add("gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "AGENTS.md"); strings.Contains(got, "Loops, $( )") {
		t.Errorf("codex does not rewrite commands, its AGENTS.md block must not say how to write them:\n%s", got)
	}
	if _, err := agents["pi"].add("gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "AGENTS.md"); strings.Contains(got, "the show tool") || !strings.Contains(got, "Loops, $( )") {
		t.Errorf("pi has no MCP tools but rewrites commands:\n%s", got)
	}
}

func TestGuideLevels(t *testing.T) {
	sandbox(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	tests := []struct {
		level           string
		wantEmpty       bool
		wantShell, hint bool
	}{
		{"full", false, true, true},
		{"short", false, false, true},
		{"off", true, false, false},
		{"typo", false, true, true}, // unknown: full, never silently off
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			t.Setenv("GASCOIGNE_GUIDE", tt.level)
			g := guide(true, true)
			if (g == "") != tt.wantEmpty {
				t.Fatalf("empty = %v:\n%s", g == "", g)
			}
			if got := strings.Contains(g, "Read and search with shell commands"); got != tt.wantShell {
				t.Errorf("shell advice = %v", got)
			}
			if got := strings.Contains(g, "gascoigne raw <id>"); got != tt.hint {
				t.Errorf("hint explanation = %v", got)
			}
		})
	}
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
	if err := saveGuideLevel("loud"); err == nil {
		t.Error("an unknown level must be refused")
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
		t.Fatal("full level writes the guide")
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
