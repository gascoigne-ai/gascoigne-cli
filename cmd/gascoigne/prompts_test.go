package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestImports(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       []string
	}{
		{"relative", "See @docs/rules.md for rules.", []string{"docs/rules.md"}},
		{"home and dot", "@~/.claude/mine.md\n@./a.md", []string{"~/.claude/mine.md", "./a.md"}},
		{"code span skipped", "not `@docs/x.md` but @docs/y.md", []string{"docs/y.md"}},
		{"fence skipped", "```\n@docs/x.md\n```\n", nil},
		{"email is no import", "mail me@example.com", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := imports(tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("imports = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClaudeSources(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	dir := filepath.Join(root, "repo")
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(home, ".claude", "CLAUDE.md"), "user rules")
	write(filepath.Join(dir, "CLAUDE.md"), "project <!-- maintainer note -->\n@docs/a.md\n")
	write(filepath.Join(dir, "docs", "a.md"), "a imports @b.md\n")
	write(filepath.Join(dir, "docs", "b.md"), "b imports @a.md back\n") // cycle
	write(filepath.Join(dir, ".claude", "skills", "x", "SKILL.md"), "---\nname: x\ndescription: does x\nother: 1\n---\nbody")
	write(filepath.Join(home, ".claude", "projects", projectKey(dir), "memory", "MEMORY.md"), "- [m](m.md)\n")

	srcs := claudeSources(dir, home)
	var got []string
	for _, s := range srcs {
		got = append(got, filepath.Base(s.Path)+" "+strings.Fields(s.Via)[0])
	}
	want := []string{"CLAUDE.md user", "CLAUDE.md project", "a.md @import", "b.md @import", "MEMORY.md memory", "SKILL.md skill"}
	if !slices.Equal(got, want) {
		t.Fatalf("sources = %q, want %q", got, want)
	}
	if strings.Contains(srcs[1].text, "maintainer") {
		t.Error("HTML comment counted; the agent strips it")
	}
	if srcs[5].text != "name: x\ndescription: does x" {
		t.Errorf("skill text = %q, want name and description only", srcs[5].text)
	}
}

func TestPromptFindings(t *testing.T) {
	line := "Never lose information: originals stay in the ledger.\n"
	srcs := []promptSource{
		{Path: "CLAUDE.md", Via: "project", text: line + "short\n", full: true, Tokens: 20},
		{Path: "big.md", Via: "@import from CLAUDE.md", text: line, full: true, Tokens: 5000},
		{Path: "SKILL.md", Via: "skill", Tokens: 200},
	}
	var got []string
	for _, f := range promptFindings(srcs, 2000, 1) {
		got = append(got, f.Kind+" "+f.Path)
	}
	want := []string{"large-import big.md", "long-description SKILL.md", "duplicate big.md"}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %q, want %q", got, want)
	}
}
