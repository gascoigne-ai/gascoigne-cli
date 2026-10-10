package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallProject(t *testing.T) {
	sandbox(t)
	exe := "/opt/bin/gascoigne"
	write(t, ".claude/settings.json", `{"model": "opus", "hooks": {"PreToolUse": [{"matcher": "Edit", "hooks": [{"type": "command", "command": "lint"}]}]}}`)
	write(t, ".codex/config.toml", "model = \"o3\"\n")
	for _, name := range []string{"claude", "cursor", "codex", "pi"} {
		if _, err := agents[name].add(exe, false); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	settings := read(t, ".claude/settings.json")
	for _, want := range []string{`"model": "opus"`, `"command": "lint"`, `"command": "/opt/bin/gascoigne hook claude"`, `"GASCOIGNE_AGENT": "claude"`} {
		if !strings.Contains(settings, want) {
			t.Errorf("claude settings lack %s:\n%s", want, settings)
		}
	}
	for file, want := range map[string]string{
		".mcp.json":                   `"args": [`,
		".cursor/hooks.json":          `hook cursor`,
		".cursor/mcp.json":            `"command": "/opt/bin/gascoigne"`,
		".codex/config.toml":          `[mcp_servers.gascoigne]`,
		".pi/extensions/gascoigne.ts": piHeader,
	} {
		if got := read(t, file); !strings.Contains(got, want) {
			t.Errorf("%s lacks %s:\n%s", file, want, got)
		}
	}
	if _, err := os.Stat(".claude/settings.json.gascoigne-bak"); err != nil {
		t.Error("changed file not backed up")
	}
}

func TestInstallTwiceChangesNothing(t *testing.T) {
	sandbox(t)
	for _, name := range []string{"claude", "cursor", "codex", "pi"} {
		if _, err := agents[name].add("gascoigne", false); err != nil {
			t.Fatal(err)
		}
		done, err := agents[name].add("gascoigne", false)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range done {
			if !strings.HasSuffix(d, "already registered") {
				t.Errorf("%s: second install changed: %s", name, d)
			}
		}
	}
	if got := read(t, ".codex/config.toml"); strings.Count(got, "[mcp_servers.gascoigne]") != 1 {
		t.Errorf("codex config:\n%s", got)
	}
}

func TestInstallGlobalUnwrapRoundTrip(t *testing.T) {
	home := sandbox(t)
	for _, name := range []string{"claude", "cursor", "codex", "pi"} {
		if _, err := agents[name].add("/opt/bin/gascoigne", true); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, f := range []string{".claude/settings.json", ".claude.json", ".cursor/hooks.json", ".cursor/mcp.json", ".codex/config.toml", ".pi/agent/extensions/gascoigne.ts"} {
		if _, err := os.Stat(filepath.Join(home, f)); err != nil {
			t.Errorf("not written: %s", f)
		}
	}
	if _, err := os.Stat(".claude"); !os.IsNotExist(err) {
		t.Error("global install touched the project")
	}
	for _, name := range []string{"claude", "cursor", "codex", "pi"} {
		if done, err := agents[name].remove(); err != nil || len(done) == 0 {
			t.Errorf("%s: unwrap done %q, err %v", name, done, err)
		}
	}
	if got := read(t, filepath.Join(home, ".claude/settings.json")); got != "{}\n" {
		t.Errorf("claude settings after unwrap: %q", got)
	}
}

func TestInstallLeavesForeignPiExtension(t *testing.T) {
	sandbox(t)
	write(t, ".pi/extensions/gascoigne.ts", "// mine\n")
	if _, err := addPi("gascoigne", false); err == nil {
		t.Fatal("overwrote a file gascoigne did not write")
	}
	if got := read(t, ".pi/extensions/gascoigne.ts"); got != "// mine\n" {
		t.Errorf("file changed: %q", got)
	}
}
