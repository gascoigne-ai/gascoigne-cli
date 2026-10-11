package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInstallProject(t *testing.T) {
	sandbox(t)
	exe := "/opt/bin/gascoigne"
	write(t, ".claude/settings.json", `{"model": "opus", "hooks": {"PreToolUse": [{"matcher": "Edit", "hooks": [{"type": "command", "command": "lint"}]}]}}`)
	write(t, ".codex/config.toml", "model = \"o3\"\n")
	for _, name := range []string{"claude", "cursor", "codex", "pi", "gemini", "opencode"} {
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
		".mcp.json":                      `"args": [`,
		".cursor/hooks.json":             `hook cursor`,
		".cursor/mcp.json":               `"command": "/opt/bin/gascoigne"`,
		".codex/config.toml":             `[mcp_servers.gascoigne]`,
		".pi/extensions/gascoigne.ts":    piHeader,
		".gemini/settings.json":          `hook gemini`,
		"opencode.json":                  `"type": "local"`,
		".opencode/plugins/gascoigne.ts": opencodeHeader,
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
	for _, name := range []string{"claude", "cursor", "codex", "pi", "gemini", "opencode"} {
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
	for _, name := range []string{"claude", "cursor", "codex", "pi", "gemini", "opencode"} {
		if _, err := agents[name].add("/opt/bin/gascoigne", true); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, f := range []string{".claude/settings.json", ".claude.json", ".cursor/hooks.json", ".cursor/mcp.json", ".codex/config.toml", ".pi/agent/extensions/gascoigne.ts",
		".gemini/settings.json", ".config/opencode/opencode.json", ".config/opencode/plugins/gascoigne.ts"} {
		if _, err := os.Stat(filepath.Join(home, f)); err != nil {
			t.Errorf("not written: %s", f)
		}
	}
	if _, err := os.Stat(".claude"); !os.IsNotExist(err) {
		t.Error("global install touched the project")
	}
	for _, name := range []string{"claude", "cursor", "codex", "pi", "gemini", "opencode"} {
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

func TestPerRunEnv(t *testing.T) {
	home := sandbox(t)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("GEMINI_CLI_SYSTEM_SETTINGS_PATH", filepath.Join(home, "no-system.json"))
	write(t, filepath.Join(home, ".gemini/settings.json"), `{"mcpServers": {"db": {"command": "pg-mcp", "args": ["--ro"]}, "web": {"httpUrl": "https://x"}}}`)
	write(t, "opencode.json", `{"mcp": {"db": {"type": "local", "command": ["pg-mcp", "--ro"]}}}`)
	env, err := geminiEnv("/opt/bin/gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	file := strings.TrimPrefix(env[0], "GEMINI_CLI_SYSTEM_SETTINGS_PATH=")
	got := read(t, file)
	for _, want := range []string{`"run_shell_command"`, `hook gemini`, `"mcp-proxy"`, `"pg-mcp"`} {
		if !strings.Contains(got, want) {
			t.Errorf("gemini run settings lack %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"web"`) {
		t.Errorf("remote server copied:\n%s", got)
	}
	env, err = opencodeEnv("/opt/bin/gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, want := range []string{"OPENCODE_CONFIG_DIR=", `"mcp-proxy"`, `"pg-mcp"`, `"instructions"`} {
		if !strings.Contains(joined, want) {
			t.Errorf("opencode env lacks %s:\n%s", want, joined)
		}
	}
	dir := strings.TrimPrefix(env[0], "OPENCODE_CONFIG_DIR=")
	if got := read(t, filepath.Join(dir, "plugins", "gascoigne.ts")); !strings.HasPrefix(got, opencodeHeader) {
		t.Errorf("plugin: %q", got)
	}
}

func TestInstallDeniesRoundTrip(t *testing.T) {
	home := sandbox(t)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	write(t, ".claude/settings.json", `{"permissions": {"deny": ["Bash(rm -rf:*)", "Glob"]}}`)
	if _, err := addClaude("gascoigne", false); err != nil {
		t.Fatal(err)
	}
	got := read(t, ".claude/settings.json")
	for _, want := range []string{`"Bash(rm -rf:*)"`, `"Glob"`, `"Grep"`, `"WebFetch"`, `"PostToolUse"`} {
		if !strings.Contains(got, want) {
			t.Errorf("settings lack %s:\n%s", want, got)
		}
	}
	if _, err := removeClaude(); err != nil {
		t.Fatal(err)
	}
	got = read(t, ".claude/settings.json")
	if !strings.Contains(got, `"Bash(rm -rf:*)"`) || !strings.Contains(got, `"Glob"`) || strings.Contains(got, "Grep") || strings.Contains(got, "WebFetch") {
		t.Errorf("unwrap should keep the user's rules (Glob was theirs) and drop gascoigne's:\n%s", got)
	}
}

func TestInstallProxyRoundTrip(t *testing.T) {
	sandbox(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GASCOIGNE_SERVER_ADDR", "")
	write(t, ".claude/settings.json", `{"env": {"FOO": "1"}}`)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	if _, err := addClaude("gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if _, err := addClaudeProxy(false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".claude/settings.json"); !strings.Contains(got, `"ANTHROPIC_BASE_URL": "http://127.0.0.1:7878"`) || !strings.Contains(got, `"FOO": "1"`) {
		t.Fatalf("settings:\n%s", got)
	}
	if _, err := removeClaude(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".claude/settings.json"); strings.Contains(got, "ANTHROPIC_BASE_URL") || !strings.Contains(got, `"FOO": "1"`) {
		t.Fatalf("after unwrap:\n%s", got)
	}
}

func TestClaudeProxyNeedsKeyOrOverride(t *testing.T) {
	tests := []struct {
		name, key, token, mode string
		ok                     bool
	}{
		{"claude.ai login", "", "", "", false},
		{"api key", "sk", "", "", true},
		{"gateway token", "", "tok", "", true},
		{"login, forced", "", "", "on", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ANTHROPIC_API_KEY", tt.key)
			t.Setenv("ANTHROPIC_AUTH_TOKEN", tt.token)
			t.Setenv("GASCOIGNE_PROXY", tt.mode)
			if got := claudeProxyOK(); got != tt.ok {
				t.Fatalf("claudeProxyOK = %v, want %v", got, tt.ok)
			}
		})
	}
	sandbox(t)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_AUTH_TOKEN", "")
	t.Setenv("GASCOIGNE_PROXY", "")
	if _, err := addClaudeProxy(false); err == nil || !strings.Contains(err.Error(), "install -proxy refused") {
		t.Fatalf("install -proxy with a login: %v", err)
	}
	if env := proxyForClaude(); env != nil {
		t.Fatalf("wrap with a login: %v", env)
	}
}

func TestClaudeProxyVar(t *testing.T) {
	clear := func(t *testing.T) {
		for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL", "ANTHROPIC_BEDROCK_BASE_URL",
			"CLAUDE_CODE_USE_BEDROCK", "CLAUDE_CODE_USE_VERTEX", "CLAUDE_CODE_USE_FOUNDRY", "AWS_BEARER_TOKEN_BEDROCK",
			"AWS_REGION", "AWS_DEFAULT_REGION", "GASCOIGNE_PROXY"} {
			t.Setenv(k, "")
		}
	}
	tests := []struct {
		name      string
		env       map[string]string
		key, path string
		note      string // substring
	}{
		{"claude.ai login", nil, "", "", "claude.ai login"},
		{"api key", map[string]string{"ANTHROPIC_API_KEY": "sk"}, "ANTHROPIC_BASE_URL", "", ""},
		{"bedrock api key", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1", "AWS_BEARER_TOKEN_BEDROCK": "b", "AWS_REGION": "eu-west-1"},
			"ANTHROPIC_BEDROCK_BASE_URL", "/bedrock/eu-west-1", ""},
		{"bedrock sigv4", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1", "AWS_REGION": "eu-west-1"}, "", "", "sign each request body"},
		{"bedrock no region", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "true", "AWS_BEARER_TOKEN_BEDROCK": "b"}, "", "", "AWS_REGION"},
		{"bedrock own gateway", map[string]string{"CLAUDE_CODE_USE_BEDROCK": "1", "AWS_BEARER_TOKEN_BEDROCK": "b", "AWS_REGION": "x", "ANTHROPIC_BEDROCK_BASE_URL": "https://gw"}, "", "", ""},
		{"vertex", map[string]string{"CLAUDE_CODE_USE_VERTEX": "1", "ANTHROPIC_API_KEY": "sk"}, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clear(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			key, path, note := claudeProxyVar()
			if key != tt.key || path != tt.path || (tt.note == "") != (note == "") || !strings.Contains(note, tt.note) {
				t.Fatalf("got %q %q %q", key, path, note)
			}
		})
	}
}

func TestInstallBedrockProxyRoundTrip(t *testing.T) {
	sandbox(t)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("GASCOIGNE_SERVER_ADDR", "")
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "b")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("ANTHROPIC_BEDROCK_BASE_URL", "")
	t.Setenv("ANTHROPIC_BASE_URL", "")
	if _, err := addClaudeProxy(false); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".claude/settings.json"); !strings.Contains(got, `"ANTHROPIC_BEDROCK_BASE_URL": "http://127.0.0.1:7878/bedrock/us-east-1"`) {
		t.Fatalf("settings:\n%s", got)
	}
	if _, err := removeClaude(); err != nil {
		t.Fatal(err)
	}
	if got := read(t, ".claude/settings.json"); strings.Contains(got, "BEDROCK") {
		t.Fatalf("after unwrap:\n%s", got)
	}
}

func TestProxyForClaudeKeepsUserGateway(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "https://gateway.example.com")
	if env := proxyForClaude(); env != nil {
		t.Fatalf("replaced the user's gateway: %v", env)
	}
}

func TestProxyForGeminiNeedsAPIKey(t *testing.T) {
	tests := []struct{ name, key, base string }{
		{"google login", "", ""},
		{"user gateway", "k", "https://gateway.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GEMINI_API_KEY", tt.key)
			t.Setenv("GOOGLE_GEMINI_BASE_URL", tt.base)
			if env := proxyForGemini(); env != nil {
				t.Fatalf("pointed gemini at the proxy: %v", env)
			}
		})
	}
}

func TestServerHostsLLMProxy(t *testing.T) {
	sandbox(t)
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = r.URL.Path + " " + string(b)
	}))
	defer upstream.Close()
	ledgerFile := filepath.Join(t.TempDir(), "l.db")
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	write(t, cfg, "extensions: { ledger: { path: "+ledgerFile+" } }\nreceivers: { httpproxy: { anthropic: "+upstream.URL+" } }\n")
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	var mu sync.Mutex
	proxy, closeProxy, err := llmProxy(t.Context(), &mu, ledgerFile, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	req := httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"m","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	proxy.ServeHTTP(httptest.NewRecorder(), req)
	if got != `/v1/messages {"model":"m","messages":[]}` {
		t.Fatalf("upstream got %q", got)
	}
}
