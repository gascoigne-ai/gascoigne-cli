package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProxyServersRoundTrip(t *testing.T) {
	root := toObject(map[string]any{"mcpServers": map[string]any{
		"github":    map[string]any{"command": "npx", "args": []string{"-y", "@modelcontextprotocol/server-github"}, "env": map[string]any{"TOKEN": "${GH}"}},
		"bare":      map[string]any{"command": "srv"},
		"remote":    map[string]any{"type": "http", "url": "https://mcp.example.com", "headers": map[string]any{"Authorization": "Bearer ${TOKEN}"}},
		"legacy":    map[string]any{"type": "sse", "url": "https://mcp.example.com/sse"},
		"untyped":   map[string]any{"url": "https://mcp.example.com/any", "headers": map[string]any{"X-Key": "${env:KEY}"}},
		"gemini":    map[string]any{"httpUrl": "https://mcp.example.com"},
		"gascoigne": map[string]any{"command": "gascoigne", "args": []string{"mcp"}},
	}})
	before := compactJSON(toMaps(root)) // restored remote entries may list their keys in another order
	r := proxyServers("gascoigne", obj(root.get("mcpServers")), nil)
	slices.Sort(r.proxied)
	slices.Sort(r.direct)
	if !slices.Equal(r.proxied, []string{"bare", "github", "legacy", "remote", "untyped"}) || len(r.direct) != 1 || !strings.HasPrefix(r.direct[0], "gemini (") {
		t.Fatalf("result %+v", r)
	}
	if got := compactJSON(toMaps(path(root, "mcpServers", "remote"))); got != `{"args":["mcp-proxy","-url","https://mcp.example.com","-header","Authorization: Bearer ${TOKEN}"],"command":"gascoigne"}` {
		t.Errorf("remote: %s", got)
	}
	if got := compactJSON(toMaps(path(root, "mcpServers", "untyped"))); got != `{"args":["mcp-proxy","-url","https://mcp.example.com/any","-auto","-header","X-Key: ${env:KEY}"],"command":"gascoigne"}` {
		t.Errorf("untyped: %s", got)
	}
	if got := compactJSON(toMaps(path(root, "mcpServers", "legacy"))); got != `{"args":["mcp-proxy","-url","https://mcp.example.com/sse","-sse"],"command":"gascoigne"}` {
		t.Errorf("legacy: %s", got)
	}
	gh := toMaps(path(root, "mcpServers", "github"))
	if got := compactJSON(gh); got != `{"args":["mcp-proxy","--","npx","-y","@modelcontextprotocol/server-github"],"command":"gascoigne","env":{"TOKEN":"${GH}"}}` {
		t.Errorf("github: %s", got)
	}
	again := proxyServers("gascoigne", obj(root.get("mcpServers")), nil)
	if len(again.proxied) != 5 || compactJSON(toMaps(path(root, "mcpServers", "github"))) != compactJSON(gh) {
		t.Errorf("second run proxied twice: %s", compactJSON(toMaps(path(root, "mcpServers", "github"))))
	}
	unproxyServers(obj(root.get("mcpServers")))
	if after := compactJSON(toMaps(root)); after != before {
		t.Errorf("unproxy did not restore:\n%s\nwant\n%s", after, before)
	}
}

func TestClaudeProxies(t *testing.T) {
	home := sandbox(t)
	cwd, _ := filepath.Abs(".")
	write(t, filepath.Join(home, ".claude.json"), `{
  "mcpServers": {"user-srv": {"command": "u"}, "shadowed": {"command": "from-user"}},
  "projects": {"`+cwd+`": {
    "mcpServers": {"shadowed": {"command": "from-local"}, "off": {"command": "x"}},
    "enabledMcpjsonServers": ["approved"],
    "disabledMcpServers": ["off"]
  }}
}`)
	write(t, ".mcp.json", `{"mcpServers": {"approved": {"command": "a"}, "pending": {"command": "p"}, "web": {"type": "sse", "url": "https://x"}, "plain": {"url": "https://y"}}}`)
	servers, r := claudeProxies("gascoigne")
	var names []string
	for n := range servers {
		names = append(names, n)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"approved", "shadowed", "user-srv"}) {
		t.Fatalf("servers %v, result %+v", names, r)
	}
	if got := compactJSON(servers["shadowed"]); !strings.Contains(got, "from-local") {
		t.Errorf("local scope should win: %s", got)
	}
	args, err := claudeArgs("gascoigne")
	if err != nil || !strings.Contains(strings.Join(args, " "), `"user-srv":{"args":["mcp-proxy","--","u"]`) {
		t.Errorf("wrap args %q, err %v", args, err)
	}
}

const codexConfig = `model = "o3"

[mcp_servers.github]
command = "npx"
args = [
  "-y", # package runner
  "@modelcontextprotocol/server-github",
]

[mcp_servers.github.env]
TOKEN = "x"

[mcp_servers."docs.site"]
command = 'docs-mcp'

[mcp_servers.remote]
url = "https://mcp.example.com"
`

func TestCodexServers(t *testing.T) {
	got := codexServers(codexConfig)
	if len(got) != 3 || got[0].name != "github" || got[0].cmd != "npx" || !slices.Equal(got[0].args, []string{"-y", "@modelcontextprotocol/server-github"}) ||
		got[1].name != "docs.site" || got[1].cmd != "docs-mcp" || !got[2].remote {
		t.Fatalf("%+v", got)
	}
	args, r := codexProxyArgs("gascoigne", got)
	want := []string{"-c", `mcp_servers.github.command="gascoigne"`, "-c", `mcp_servers.github.args=["mcp-proxy", "--", "npx", "-y", "@modelcontextprotocol/server-github"]`}
	if !slices.Equal(args, want) || len(r.direct) != 2 {
		t.Errorf("args %q\nresult %+v", args, r)
	}
}

func TestProxyCodexTOMLRoundTrip(t *testing.T) {
	out, r := proxyCodexTOML("gascoigne", codexConfig)
	if len(r.proxied) != 2 || !strings.Contains(out, `args = ["mcp-proxy", "--", "docs-mcp"]`) || !strings.Contains(out, "TOKEN = \"x\"") {
		t.Fatalf("result %+v:\n%s", r, out)
	}
	if again, _ := proxyCodexTOML("gascoigne", out); again != out {
		t.Errorf("second run changed:\n%s", again)
	}
	back, names := unproxyCodexTOML(out)
	if len(names) != 2 {
		t.Fatalf("restored %v", names)
	}
	for i, s := range codexServers(back) {
		if o := codexServers(codexConfig)[i]; s.cmd != o.cmd || !slices.Equal(s.args, o.args) {
			t.Errorf("%s: got %q %q, want %q %q", s.name, s.cmd, s.args, o.cmd, o.args)
		}
	}
	if strings.Contains(back, "args = []") || strings.Contains(back, "gascoigne") {
		t.Errorf("leftovers:\n%s", back)
	}
}

func TestInstallProxiesAndUnwrapRestores(t *testing.T) {
	sandbox(t)
	mcp := `{"mcpServers": {"github": {"command": "npx", "args": ["-y", "srv"]}}}`
	write(t, ".mcp.json", mcp)
	write(t, ".codex/config.toml", codexConfig)
	write(t, ".cursor/mcp.json", mcp)
	for _, name := range []string{"claude", "codex", "cursor"} {
		done, err := agents[name].add("gascoigne", false)
		if err != nil || !strings.Contains(strings.Join(done, "\n"), "proxied MCP servers github") {
			t.Fatalf("%s: %q, err %v", name, done, err)
		}
	}
	if got := read(t, ".mcp.json"); !strings.Contains(got, `"mcp-proxy"`) {
		t.Fatalf(".mcp.json:\n%s", got)
	}
	for _, name := range []string{"claude", "codex", "cursor"} {
		if _, err := agents[name].remove(); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{".mcp.json", ".cursor/mcp.json"} {
		if got := read(t, f); strings.Contains(got, "gascoigne") || !strings.Contains(got, `"command": "npx"`) {
			t.Errorf("%s after unwrap:\n%s", f, got)
		}
	}
	if got := read(t, ".codex/config.toml"); strings.Contains(got, "gascoigne") {
		t.Errorf("codex after unwrap:\n%s", got)
	}
}
