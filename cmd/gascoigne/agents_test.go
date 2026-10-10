package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestClaudeArgs(t *testing.T) {
	args, err := claudeArgs("/opt/gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 4 || args[0] != "--mcp-config" || args[2] != "--settings" {
		t.Fatalf("got %q", args)
	}
	var mcp, settings map[string]any
	if json.Unmarshal([]byte(args[1]), &mcp) != nil || json.Unmarshal([]byte(args[3]), &settings) != nil {
		t.Fatalf("invalid JSON: %q", args)
	}
	if !strings.Contains(args[3], `"command":"/opt/gascoigne hook claude"`) || !strings.Contains(args[3], `"matcher":"Bash"`) {
		t.Fatalf("settings: %s", args[3])
	}
	if !strings.Contains(args[3], `"permissions":{"deny":["Grep","Glob"]}`) {
		t.Fatalf("settings should deny Grep and Glob: %s", args[3])
	}
	if strings.Contains(claudeInstall("/opt/gascoigne"), `"permissions"`) {
		t.Fatal("install config must not deny tools: unwrap cannot tell the rule from the user's own")
	}
}

func TestCodexArgs(t *testing.T) {
	args, err := codexArgs("/opt/my tools/gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-c", `mcp_servers.gascoigne={command="/opt/my tools/gascoigne",args=["mcp"]}`,
		"-c", `hooks.PostToolUse=[{matcher="^Bash$",hooks=[{type="command",command="'/opt/my tools/gascoigne' hook codex"}]}]`,
	}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(args, "\n"))
	}
}

func TestPiArgs(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	args, err := piArgs("/opt/gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	ext, err := os.ReadFile(args[1])
	if err != nil {
		t.Fatal(err)
	}
	if args[0] != "--extension" || !strings.Contains(string(ext), `execFileSync("/opt/gascoigne", ["hook", "pi"]`) {
		t.Fatalf("args %q, extension:\n%s", args, ext)
	}
}

func TestInstallSnippets(t *testing.T) {
	for name, a := range agents {
		t.Run(name, func(t *testing.T) {
			if got := a.install("/opt/gascoigne"); !strings.Contains(got, "/opt/gascoigne") {
				t.Fatalf("no gascoigne path in:\n%s", got)
			}
		})
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/bin/gascoigne": "/usr/bin/gascoigne",
		"/opt/my tools/g":    "'/opt/my tools/g'",
		"/it's":              `'/it'\''s'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
