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
