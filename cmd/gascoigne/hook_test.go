package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWrapCommand(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"known tool", "go test ./...", "gascoigne exec -- go test ./..."},
		{"after cd", "cd api && go test ./...", "cd api && gascoigne exec -- go test ./..."},
		{"env kept outside", "CGO_ENABLED=0 go build ./...", "CGO_ENABLED=0 gascoigne exec -- go build ./..."},
		{"stderr merge", "npm install 2>&1", "gascoigne exec -- npm install 2>&1"},
		{"quoted args", `git log --format="%h %s"`, `gascoigne exec -- git log --format="%h %s"`},
		{"quiet command", "echo hi", ""},
		{"unknown tool", "./scripts/report.py --json", "gascoigne exec -- ./scripts/report.py --json"},
		{"custom binary", "mytool list", "gascoigne exec -- mytool list"},
		{"repl", "python3", ""},
		{"script", "python3 gen.py", "gascoigne exec -- python3 gen.py"},
		{"quiet behind sudo", "sudo rm -rf build", ""},
		{"env only", "FOO=1", ""},
		{"builtin", "cd api", ""},
		{"tail pipe", "go test ./... | tail -5", "gascoigne exec -- go test ./... | tail -5"},
		{"head pipe", "git log -n 50 2>&1 | head -n 20", "gascoigne exec -attr hint=top -- git log -n 50 2>&1 | head -n 20"},
		{"head and tail", "cd api && go test ./... | head -40 | tail -5", "cd api && gascoigne exec -attr hint=top -- go test ./... | head -40 | tail -5"},
		{"grep pipe", "go test ./... | grep FAIL", ""},
		{"followed tail pipe", "go test ./... | tail -f", ""},
		{"head with file", "go test ./... | head out.txt", ""},
		{"or list", "go test ./... || true", ""},
		{"sequence", "go vet ./...; go test ./...", "gascoigne exec -- go vet ./...; gascoigne exec -- go test ./..."},
		{"every command", "ls docs; cat a.md && git log -5", "gascoigne exec -- ls docs; gascoigne exec -- cat a.md && gascoigne exec -- git log -5"},
		{"hint first only before head", "git status && git log | head -5", "gascoigne exec -- git status && gascoigne exec -attr hint=top -- git log | head -5"},
		{"streaming segment skipped", "npm run dev; go test ./...", "npm run dev; gascoigne exec -- go test ./..."},
		{"tsc watch", "npx tsc -w -p .", "gascoigne exec -watch -- npx tsc -w -p ."},
		{"webpack dev server", "webpack serve --mode development", "gascoigne exec -watch -- webpack serve --mode development"},
		{"vite build watch", "vite build --watch", "gascoigne exec -watch -- vite build --watch"},
		{"vite dev server", "npx vite --port 5173", ""},
		{"watch flag of another family", "jest --watch", ""},
		{"quiet segments kept", "echo start; go test ./...; echo end", "echo start; gascoigne exec -- go test ./...; echo end"},
		{"conditional", "if [ -f go.mod ]; then go test ./...; fi", ""},
		{"while loop", "while true; do go test ./...; done", ""},
		{"grouping", "{ go vet ./...; go test ./...; }", ""},
		{"mixed lists", "cd api; make gen && go test ./...", "cd api; gascoigne exec -- make gen && gascoigne exec -- go test ./..."},
		{"quoted ;", `git log --format="%h;%s"`, `gascoigne exec -- git log --format="%h;%s"`},
		{"loop", "for d in a b; do go test ./$d; done", ""},
		{"path named gascoigne", "cd ~/src/gascoigne && go test ./...", "cd ~/src/gascoigne && gascoigne exec -- go test ./..."},
		{"local binary", "./bin/gascoigne gain", ""},
		{"redirect", "go test ./... > out.txt", ""},
		{"subshell", "kubectl get pods -n $(cat ns)", ""},
		{"background", "go test ./... &", ""},
		{"already wrapped", "gascoigne exec -- go test ./...", ""},
		{"raw requested", "GASCOIGNE_RAW=1 go test ./...", ""},
		{"quoted &&", `echo "a && go test ./..."`, ""},
		{"followed logs", "kubectl logs -f api", ""},
		{"watch", "kubectl get pods --watch", ""},
		{"dev server", "npm run dev", ""},
		{"compose up", "docker compose up", ""},
		{"multiline", "go test ./...\ngo vet ./...", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := wrapCommand(tt.in, "gascoigne exec")
			if want := tt.want != ""; ok != want {
				t.Fatalf("ok = %v, got %q", ok, got)
			}
			if ok && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRewriteReplies(t *testing.T) {
	in := hookInput{ToolInput: map[string]any{"command": "go test ./...", "description": "run tests"}}
	tests := []struct {
		agent, want string
	}{
		{"claude", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"gascoigne exec -- go test ./...","description":"run tests"}}}`},
		{"cursor", `{"permission":"allow","updated_input":{"command":"gascoigne exec -- go test ./...","description":"run tests"}}`},
		{"pi", `{"command":"gascoigne exec -- go test ./..."}`},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			reply, err := agents[tt.agent].hook(t.Context(), &common{}, in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			if got := compactJSON(reply); got != tt.want {
				t.Fatalf("got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestRewriteLeavesAlone(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{"unknown tool", map[string]any{"command": "echo hi"}},
		{"background", map[string]any{"command": "go test ./...", "run_in_background": true}},
		{"no command", map[string]any{"file_path": "/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply, _ := agents["claude"].hook(t.Context(), &common{}, hookInput{ToolInput: tt.input}, "gascoigne exec")
			if reply != nil {
				t.Fatalf("claude got %v", reply)
			}
			reply, _ = agents["cursor"].hook(t.Context(), &common{}, hookInput{ToolInput: tt.input}, "gascoigne exec")
			if got := compactJSON(reply); got != `{"permission":"allow"}` {
				t.Fatalf("cursor got %s", got)
			}
		})
	}
}

func TestShellOutput(t *testing.T) {
	tests := []struct {
		raw, text string
		code      int
		ok        bool
	}{
		{`"out\n"`, "out\n", 0, true},
		{`{"output":"FAIL\n","exit_code":1}`, "FAIL\n", 1, true},
		{`{"other":1}`, "", 0, false},
		{``, "", 0, false},
	}
	for _, tt := range tests {
		text, code, ok := shellOutput(json.RawMessage(tt.raw))
		if text != tt.text || code != tt.code || ok != tt.ok {
			t.Errorf("%s: got %q %d %v", tt.raw, text, code, ok)
		}
	}
}

func TestCodexOptimizesOutput(t *testing.T) {
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "ledger.db"))
	t.Setenv("GASCOIGNE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fixture, err := os.ReadFile("../../testdata/eval/go/test-fail-v.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := json.Marshal(map[string]any{"output": string(fixture), "exit_code": 1})
	in := hookInput{ToolInput: map[string]any{"command": "go test -v ./..."}, ToolResponse: resp}
	c := &common{attrs: attrFlag{}}
	reply, err := agents["codex"].hook(t.Context(), c, in, "gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	m, _ := reply.(map[string]any)
	reason, _ := m["reason"].(string)
	if m["decision"] != "block" || !strings.HasPrefix(reason, "exit code 1\n") || !strings.Contains(reason, "full: gascoigne raw ") {
		t.Fatalf("got %v", reply)
	}
	if len(reason) >= len(fixture) {
		t.Fatalf("not smaller: %d >= %d", len(reason), len(fixture))
	}

	in.ToolInput["command"] = "gascoigne exec -- go test -v ./..."
	if reply, _ := agents["codex"].hook(t.Context(), c, in, "gascoigne"); reply != nil {
		t.Fatalf("wrapped command optimized twice: %v", reply)
	}
}

func TestExecPrefix(t *testing.T) {
	tests := []struct {
		name, envAgent, envSession, hookSession, want string
	}{
		{"wrapped: env says it all", "claude", "claude-20261010", "uuid-1", "gascoigne exec"},
		{"installed for good: hook names the agent", "", "", "uuid-1", "gascoigne exec -agent cursor -session uuid-1"},
		{"no session from the hook", "", "", "", "gascoigne exec -agent cursor"},
		{"odd session id quoted", "", "", "a b", "gascoigne exec -agent cursor -session 'a b'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GASCOIGNE_AGENT", tt.envAgent)
			t.Setenv("GASCOIGNE_SESSION", tt.envSession)
			if got := execPrefix("gascoigne", "cursor", tt.hookSession); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRewriteBackgroundWatch(t *testing.T) {
	in := hookInput{ToolInput: map[string]any{"command": "npx tsc -w", "run_in_background": true}}
	reply, err := agents["claude"].hook(t.Context(), &common{}, in, "gascoigne exec")
	if err != nil {
		t.Fatal(err)
	}
	if got := compactJSON(reply); !strings.Contains(got, `"command":"gascoigne exec -watch -- npx tsc -w"`) {
		t.Fatalf("got %s", got)
	}
}
