package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox gives a test its own home, cache and project dir, and hides agent CLIs.
func sandbox(t *testing.T) string {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("GASCOIGNE_PROXY", "off") // never spawn the test binary as a proxy
	t.Setenv("PATH", "")
	t.Chdir(t.TempDir())
	return home
}

func write(t *testing.T, file, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRemoveClaudeKeepsOtherSettings(t *testing.T) {
	home := sandbox(t)
	settings := filepath.Join(home, ".claude/settings.json")
	write(t, settings, `{
  "model": "opus",
  "env": {"GASCOIGNE_AGENT": "claude", "FOO": "1"},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [
        {"type": "command", "command": "/opt/bin/gascoigne hook claude"},
        {"type": "command", "command": "lint && check"}
      ]},
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "gascoigne hook claude"}]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "notify"}]}]
  }
}
`)
	done, err := removeClaude()
	if err != nil || len(done) != 1 || !strings.Contains(done[0], "PreToolUse hook, GASCOIGNE_AGENT env") {
		t.Fatalf("done %q, err %v", done, err)
	}
	want := `{
  "model": "opus",
  "env": {
    "FOO": "1"
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "lint && check"
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "notify"
          }
        ]
      }
    ]
  }
}
`
	if got := read(t, settings); got != want {
		t.Fatalf("got:\n%s", got)
	}
	if !strings.Contains(read(t, settings+".gascoigne-bak"), "gascoigne hook claude") {
		t.Fatal("no backup of the original")
	}
}

func TestRemoveRoundTrip(t *testing.T) {
	home := sandbox(t)
	exe := "/opt/bin/gascoigne"
	// What wrap -print shows, as a user would paste it in.
	write(t, filepath.Join(home, ".claude/settings.json"), compactJSON(claudeSettings(exe)))
	write(t, ".mcp.json", compactJSON(mcpServers(exe)))
	write(t, filepath.Join(home, ".cursor/hooks.json"), strings.SplitN(cursorInstall(exe), "# merge into ~/.cursor/mcp.json\n", 2)[0][len("# merge into ~/.cursor/hooks.json\n"):])
	write(t, filepath.Join(home, ".cursor/mcp.json"), compactJSON(mcpServers(exe)))
	write(t, filepath.Join(home, ".codex/config.toml"), "model = \"o3\"\n\n"+codexInstall(exe))
	write(t, filepath.Join(home, ".pi/agent/extensions/gascoigne.ts"), piExtension(exe))

	for _, name := range []string{"claude", "cursor", "codex", "pi"} {
		done, err := agents[name].remove()
		if err != nil || len(done) == 0 {
			t.Fatalf("%s: done %q, err %v", name, done, err)
		}
	}
	for file, want := range map[string]string{
		filepath.Join(home, ".claude/settings.json"): "{}\n",
		".mcp.json": "{}\n",
		filepath.Join(home, ".cursor/hooks.json"): "{\n  \"version\": 1\n}\n",
		filepath.Join(home, ".cursor/mcp.json"):   "{}\n",
	} {
		if got := read(t, file); got != want {
			t.Errorf("%s: got %q", file, got)
		}
	}
	if got := read(t, filepath.Join(home, ".codex/config.toml")); strings.Contains(got, "gascoigne") || !strings.Contains(got, `model = "o3"`) {
		t.Errorf("codex config:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".pi/agent/extensions/gascoigne.ts")); !os.IsNotExist(err) {
		t.Error("pi extension still there")
	}
	if done, _ := removeClaude(); len(done) != 0 {
		t.Errorf("second run changed again: %q", done)
	}
}

func TestRemoveCodexTOMLKeepsOtherHandlers(t *testing.T) {
	in := `[[hooks.PostToolUse]]
matcher = "^Bash$"

[[hooks.PostToolUse.hooks]]
type = "command"
command = "audit.sh"

[[hooks.PostToolUse.hooks]]
type = "command"
command = "/opt/bin/gascoigne hook codex"

[profiles.fast]
model = "mini"
`
	got, what := removeCodexTOML(in)
	want := `[[hooks.PostToolUse]]
matcher = "^Bash$"

[[hooks.PostToolUse.hooks]]
type = "command"
command = "audit.sh"

[profiles.fast]
model = "mini"
`
	if got != want || len(what) != 1 {
		t.Fatalf("what %q, got:\n%s", what, got)
	}
}

func TestRemovePiOnlyOurExtension(t *testing.T) {
	home := sandbox(t)
	other := filepath.Join(home, ".pi/agent/extensions/gascoigne.ts")
	write(t, other, "// my own extension that happens to share the name\n")
	if done, _ := removePi(); len(done) != 0 {
		t.Fatalf("removed a foreign file: %q", done)
	}
	if read(t, other) == "" {
		t.Fatal("file emptied")
	}
}
