package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPostToolUse(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "l.db"))
	t.Setenv("GASCOIGNE_CONFIG", filepath.Join(t.TempDir(), "none.yaml"))
	os.WriteFile(os.Getenv("GASCOIGNE_CONFIG"), []byte("{}\n"), 0o644)
	data, err := os.ReadFile("../../testdata/eval/go/test-panic-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	trace := string(data)
	resp := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	tests := []struct {
		name  string
		in    hookInput
		field func(map[string]any) string // where the optimized text lands; nil: no reply
	}{
		{"bash with a pipe", hookInput{ToolName: "Bash", ToolInput: map[string]any{"command": "go test ./... | cat"},
			ToolResponse: resp(map[string]any{"stdout": trace, "stderr": "", "interrupted": false, "isImage": false})},
			func(o map[string]any) string { return o["stdout"].(string) }},
		{"bash already optimized", hookInput{ToolName: "Bash", ToolInput: map[string]any{"command": "gascoigne exec -- go test"},
			ToolResponse: resp(map[string]any{"stdout": "x\n[gascoigne: optimized 10→2 tokens · full: gascoigne raw ab]\n"})}, nil},
		{"grep content", hookInput{ToolName: "Grep", ToolInput: map[string]any{"pattern": "TODO"},
			ToolResponse: resp(map[string]any{"mode": "content", "numFiles": 2, "filenames": []string{}, "content": strings.Repeat("a/very/long/path/to/file.go:12:// TODO fix this thing later\n", 60)})},
			func(o map[string]any) string { return o["content"].(string) }},
		{"grep file list", hookInput{ToolName: "Grep", ToolResponse: resp(map[string]any{"mode": "files_with_matches", "filenames": []string{"a"}})}, nil},
		{"mcp blocks", hookInput{ToolName: "mcp__db__query",
			ToolResponse: resp([]any{map[string]any{"type": "text", "text": trace}, map[string]any{"type": "image", "data": "x"}})}, nil},
		{"gascoigne's own tools", hookInput{ToolName: "mcp__gascoigne__fetch", ToolResponse: resp(trace)}, nil},
		{"read stays", hookInput{ToolName: "Read", ToolResponse: resp(map[string]any{"type": "text", "file": map[string]any{"content": trace}})}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.in.HookEventName = "PostToolUse"
			reply, err := agents["claude"].hook(t.Context(), &common{attrs: attrFlag{}}, tt.in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			if tt.name == "mcp blocks" {
				out := reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["updatedToolOutput"].([]any)
				if text := out[0].(map[string]any)["text"].(string); !strings.Contains(text, "gascoigne raw") || out[1].(map[string]any)["type"] != "image" {
					t.Fatalf("got %v", out)
				}
				return
			}
			if tt.field == nil {
				if reply != nil {
					t.Fatalf("replied %v", reply)
				}
				return
			}
			out := reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["updatedToolOutput"].(map[string]any)
			if text := tt.field(out); !strings.Contains(text, "· full: gascoigne raw ") || len(text) >= len(tt.in.ToolResponse) {
				t.Fatalf("not optimized: %.200s", text)
			}
		})
	}
}

func TestResultHooksOtherAgents(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "l.db"))
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	data, _ := os.ReadFile("../../testdata/eval/go/test-panic-trace.txt")
	mcp, _ := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(data)}}})
	gem, _ := json.Marshal(map[string]any{"llmContent": string(data), "returnDisplay": "x"})
	tests := []struct {
		agent string
		in    hookInput
		key   string // reply field holding the optimized text
	}{
		{"codex", hookInput{HookEventName: "PostToolUse", ToolName: "mcp__db__query", ToolResponse: mcp}, "reason"},
		{"gemini", hookInput{HookEventName: "AfterTool", ToolName: "mcp_db_query", ToolResponse: gem}, "reason"},
		{"gemini", hookInput{HookEventName: "AfterTool", ToolName: "run_shell_command", ToolInput: map[string]any{"command": "go test ./... | cat"}, ToolResponse: gem}, "reason"},
		{"gemini", hookInput{HookEventName: "AfterTool", ToolName: "mcp_gascoigne_fetch", ToolResponse: gem}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.agent+" "+tt.in.ToolName, func(t *testing.T) {
			reply, err := agents[tt.agent].hook(t.Context(), &common{attrs: attrFlag{}}, tt.in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			if tt.key == "" {
				if reply != nil {
					t.Fatalf("replied %v", reply)
				}
				return
			}
			got, _ := reply.(map[string]any)[tt.key].(string)
			if !strings.Contains(got, "gascoigne raw") || len(got) >= len(data) {
				t.Fatalf("not optimized: %.200s", got)
			}
		})
	}
}
