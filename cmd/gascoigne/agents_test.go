package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeArgs(t *testing.T) {
	sandbox(t)
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
	if !strings.Contains(args[3], `"SessionStart":[{"hooks":[{"command":"/opt/gascoigne hook claude"`) {
		t.Fatalf("settings should add the guide at session start: %s", args[3])
	}
	if !strings.Contains(args[3], `"permissions":{"deny":["Grep","Glob","WebFetch"]}`) {
		t.Fatalf("settings should deny Grep, Glob and WebFetch: %s", args[3])
	}
	if strings.Contains(claudeInstall("/opt/gascoigne"), `"permissions"`) {
		t.Fatal("install config must not deny tools: unwrap cannot tell the rule from the user's own")
	}
}

func TestCodexArgs(t *testing.T) {
	home := sandbox(t)
	args, err := codexArgs("/opt/my tools/gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-c", `mcp_servers.gascoigne={command="/opt/my tools/gascoigne",args=["mcp"]}`,
		"-c", `hooks.PostToolUse=[{matcher="^Bash$|^mcp__.*",hooks=[{type="command",command="'/opt/my tools/gascoigne' hook codex"}]}]`,
		"-c", `hooks.UserPromptSubmit=[{hooks=[{type="command",command="'/opt/my tools/gascoigne' hook codex"}]}]`,
		"-c", "developer_instructions=" + tomlQuote(guide(true)),
	}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got\n%s", strings.Join(args, "\n"))
	}
	// The user's own instructions are not replaced.
	write(t, filepath.Join(home, ".codex/config.toml"), "developer_instructions = \"be brief\"\n")
	args, _ = codexArgs("gascoigne")
	if strings.Contains(strings.Join(args, " "), "developer_instructions") {
		t.Fatalf("replaced the user's instructions: %q", args)
	}
}

func TestClaudeSessionStartGuide(t *testing.T) {
	reply, err := agents["claude"].hook(t.Context(), &common{}, hookInput{HookEventName: "SessionStart"}, "gascoigne exec")
	if err != nil || !strings.Contains(compactJSON(reply), `"additionalContext":"gascoigne shrinks`) {
		t.Fatalf("reply %s, err %v", compactJSON(reply), err)
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

// decodedReply is a hook reply as the agent reads it: the JSON the hook
// prints, decoded, so the test does not depend on the Go type or key order.
func decodedReply(t *testing.T, reply any) map[string]any {
	t.Helper()
	if reply == nil {
		return nil
	}
	b, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// Each agent's reply to a shell call, pinned as decoded JSON. Tool input keys
// the hook does not know (timeout) must reach the agent as they came.
func TestClaudeRewriteInAutoMode(t *testing.T) {
	input := map[string]any{"command": "go test ./..."}
	for _, tt := range []struct{ mode, decision string }{
		{"auto", "allow"}, {"default", ""}, {"plan", ""}, {"acceptEdits", ""}, {"", ""},
	} {
		t.Run("mode "+tt.mode, func(t *testing.T) {
			reply, err := agents["claude"].hook(t.Context(), &common{}, hookInput{ToolInput: input, PermissionMode: tt.mode}, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			out, _ := decodedReply(t, reply)["hookSpecificOutput"].(map[string]any)
			if got, _ := out["permissionDecision"].(string); got != tt.decision {
				t.Fatalf("permissionDecision %q, want %q", got, tt.decision)
			}
		})
	}
	t.Run("unchanged call gets no decision", func(t *testing.T) {
		reply, err := agents["claude"].hook(t.Context(), &common{}, hookInput{ToolInput: map[string]any{"command": "echo hi"}, PermissionMode: "auto"}, "gascoigne exec")
		if err != nil || reply != nil {
			t.Fatalf("reply %#v err %v, want nil", reply, err)
		}
	})
}

func TestHookReplyShapes(t *testing.T) {
	rewritten := "gascoigne exec -attr query=tests -- go test ./..."
	input := map[string]any{"command": "go test ./...", "description": "run tests", "timeout": float64(5000)}
	untouched := map[string]any{"command": "echo hi"}
	tests := []struct {
		name, agent string
		input       map[string]any
		want        map[string]any // nil: no reply
	}{
		{"claude rewrite", "claude", input, map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": "PreToolUse",
			"updatedInput":  map[string]any{"command": rewritten, "description": "run tests", "timeout": float64(5000)}}}},
		{"claude unchanged", "claude", untouched, nil},
		{"cursor rewrite", "cursor", input, map[string]any{"permission": "allow",
			"updated_input": map[string]any{"command": rewritten, "description": "run tests", "timeout": float64(5000)}}},
		{"cursor unchanged", "cursor", untouched, map[string]any{"permission": "allow"}},
		{"pi rewrite", "pi", input, map[string]any{"command": rewritten}},
		{"pi unchanged", "pi", untouched, nil},
		{"gemini rewrite", "gemini", input, map[string]any{"hookSpecificOutput": map[string]any{
			"tool_input": map[string]any{"command": rewritten}}}},
		{"gemini unchanged", "gemini", untouched, nil},
		{"opencode rewrite", "opencode", input, map[string]any{"command": rewritten}},
		{"opencode unchanged", "opencode", untouched, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply, err := agents[tt.agent].hook(t.Context(), &common{}, hookInput{ToolInput: tt.input}, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			got := decodedReply(t, reply)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v\nwant %v", got, tt.want)
			}
			if tt.want == nil && reply != nil {
				t.Fatalf("no reply must be a nil interface, got %#v", reply)
			}
		})
	}
}

// Claude's PostToolUse reply wraps the replaced result, whatever its shape.
func TestPostToolUseReplyShape(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "l.db"))
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	data, err := os.ReadFile("../../testdata/eval/go/test-panic-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := json.Marshal(map[string]any{"stdout": string(data), "stderr": "", "interrupted": false, "isImage": false})
	in := hookInput{HookEventName: "PostToolUse", ToolName: "Bash", ToolInput: map[string]any{"command": "go test ./... | cat"}, ToolResponse: resp}
	reply, err := agents["claude"].hook(t.Context(), &common{attrs: attrFlag{}}, in, "gascoigne exec")
	if err != nil {
		t.Fatal(err)
	}
	got := decodedReply(t, reply)
	if len(got) != 1 {
		t.Fatalf("want only hookSpecificOutput, got %v", got)
	}
	special, _ := got["hookSpecificOutput"].(map[string]any)
	out, _ := special["updatedToolOutput"].(map[string]any)
	if len(special) != 2 || special["hookEventName"] != "PostToolUse" || out["stderr"] != "" || out["isImage"] != false ||
		!strings.Contains(out["stdout"].(string), "[raw:") {
		t.Fatalf("got %v", got)
	}
}

// The replies that replace a tool result after it ran, pinned as decoded JSON
// with exactly the keys the agent reads.
func TestResultReplyShapes(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "l.db"))
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	data, err := os.ReadFile("../../testdata/eval/go/test-panic-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	mcp, _ := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(data)}}})
	plain, _ := json.Marshal(string(data))
	tests := []struct {
		agent    string
		in       hookInput
		key, val string
	}{
		{"codex", hookInput{HookEventName: "PostToolUse", ToolName: "mcp__db__query", ToolResponse: mcp}, "decision", "block"},
		{"gemini", hookInput{HookEventName: "AfterTool", ToolName: "mcp_db_query", ToolResponse: mcp}, "decision", "deny"},
		{"opencode", hookInput{HookEventName: "PostToolUse", ToolName: "db_query", ToolResponse: plain}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			reply, err := agents[tt.agent].hook(t.Context(), &common{attrs: attrFlag{}}, tt.in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			got := decodedReply(t, reply)
			textKey := "reason"
			if tt.key == "" {
				textKey = "output"
			}
			text, _ := got[textKey].(string)
			if !strings.Contains(text, "[raw:") {
				t.Fatalf("not optimized: %v", got)
			}
			want := map[string]any{textKey: text}
			if tt.key != "" {
				want[tt.key] = tt.val
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got keys %v, want %v", got, want)
			}
		})
	}
}

// A cancelled ctx stops wrap before the agent starts.
func TestWrapCancelledContextDoesNotStartAgent(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	bin := t.TempDir()
	marker := filepath.Join(bin, "ran")
	write(t, filepath.Join(bin, "claude"), "#!/bin/sh\n: > "+marker+"\n")
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cmdWrap(ctx, []string{"claude"}); err == nil {
		t.Fatal("cmdWrap with a cancelled ctx returned nil")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context canceled", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("agent ran despite a cancelled ctx")
	}
}
