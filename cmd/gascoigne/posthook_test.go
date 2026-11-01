package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
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

// An image Read comes back as {type: image, file: {base64, type, originalSize,
// dimensions}} (the toolUseResult Claude Code writes to its transcript). Above
// max_image_pixels the hook shrinks file.base64, keeps every other key, and
// leaves the original in the ledger; text Reads and small images stay as they are.
func TestPostToolUseReadImage(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	db := filepath.Join(t.TempDir(), "l.db")
	t.Setenv("GASCOIGNE_LEDGER", db)
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	shot, err := os.ReadFile("../../docs/images/dashboard-light.png") // 1600×1319
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(shot)
	read := func(file map[string]any) hookInput {
		resp, _ := json.Marshal(map[string]any{"type": "image", "file": file})
		return hookInput{HookEventName: "PostToolUse", ToolName: "Read", ToolResponse: resp}
	}
	big := map[string]any{"base64": b64, "type": "image/png", "originalSize": len(shot),
		"dimensions": map[string]any{"originalWidth": 1600, "originalHeight": 1319, "displayWidth": 1600, "displayHeight": 1319}}
	small := map[string]any{"base64": "iVBORw0KGgo=", "type": "image/png", "originalSize": 8}
	for _, tt := range []struct {
		name   string
		in     hookInput
		shrunk bool
	}{
		{"big image", read(big), true},
		{"small image", read(small), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reply, err := agents["claude"].hook(t.Context(), &common{attrs: attrFlag{}}, tt.in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			if !tt.shrunk {
				if reply != nil {
					t.Fatalf("replied %v", reply)
				}
				return
			}
			out := reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["updatedToolOutput"].(map[string]any)
			file := out["file"].(map[string]any)
			data := file["base64"].(string)
			raw, _ := base64.StdEncoding.DecodeString(data)
			got, _, err := image.DecodeConfig(bytes.NewReader(raw))
			if err != nil || got.Width*got.Height > 600000 || len(data) >= len(b64) {
				t.Fatalf("not shrunk: %dx%d, %d of %d bytes (%v)", got.Width, got.Height, len(data), len(b64), err)
			}
			dims := file["dimensions"].(map[string]any)
			if out["type"] != "image" || file["type"] != "image/png" || file["originalSize"] != float64(len(shot)) ||
				dims["originalWidth"] != float64(1600) || dims["displayWidth"] != got.Width || dims["displayHeight"] != got.Height {
				t.Fatalf("shape changed: %v", file)
			}
			again, _ := agents["claude"].hook(t.Context(), &common{attrs: attrFlag{}}, tt.in, "gascoigne exec")
			if again.(map[string]any)["hookSpecificOutput"].(map[string]any)["updatedToolOutput"].(map[string]any)["file"].(map[string]any)["base64"] != data {
				t.Fatal("not deterministic")
			}
		})
	}
	l := ledger.New(&ledger.Config{Path: db})
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	defer l.Shutdown(t.Context())
	entries, err := l.Recent(t.Context(), component.StatsQuery{}, 0, 10)
	if err != nil || len(entries) == 0 || entries[0].Route != "image" || entries[0].RawID == "" {
		t.Fatalf("no image event: %v (%v)", entries, err)
	}
	if _, p, err := l.GetRaw(t.Context(), entries[0].RawID); err != nil || p.Text() != b64 {
		t.Fatalf("original not kept: %v", err)
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
	plain, _ := json.Marshal(string(data))
	tests := []struct {
		agent string
		in    hookInput
		key   string // reply field holding the optimized text
	}{
		{"opencode", hookInput{HookEventName: "PostToolUse", ToolName: "db_query", ToolResponse: plain}, "output"},
		{"opencode", hookInput{HookEventName: "PostToolUse", ToolName: "bash", ToolInput: map[string]any{"command": "go test ./... | cat"}, ToolResponse: plain}, "output"},
		{"opencode", hookInput{HookEventName: "PostToolUse", ToolName: "bash", ToolInput: map[string]any{"command": "gascoigne raw abc"}, ToolResponse: plain}, ""},
		{"opencode", hookInput{HookEventName: "PostToolUse", ToolName: "read", ToolResponse: plain}, ""},
		{"opencode", hookInput{HookEventName: "PostToolUse", ToolName: "gascoigne_fetch", ToolResponse: plain}, ""},
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

// The MCP review counts calls to servers that stay direct from the
// events their hooked results leave: every call, big or small, must show the
// tool's name as its command.
func TestHookedMCPCallsLeaveEvents(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	db := filepath.Join(t.TempDir(), "l.db")
	t.Setenv("GASCOIGNE_LEDGER", db)
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	data, _ := os.ReadFile("../../testdata/eval/go/test-panic-trace.txt")
	text := func(s string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": s}}})
		return b
	}
	calls := []hookInput{
		{HookEventName: "PostToolUse", ToolName: "mcp__db__query", ToolResponse: text(string(data))},
		{HookEventName: "PostToolUse", ToolName: "mcp__db__query", ToolResponse: text("ok")},
		{HookEventName: "PostToolUse", ToolName: "mcp__mail__search", ToolResponse: text("none")},
	}
	for _, in := range calls {
		if _, err := agents["codex"].hook(t.Context(), &common{attrs: attrFlag{}}, in, "gascoigne exec"); err != nil {
			t.Fatal(err)
		}
	}
	l := ledger.New(&ledger.Config{Path: db})
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	defer l.Shutdown(t.Context())
	entries, err := l.Recent(t.Context(), component.StatsQuery{}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, e := range entries {
		got[e.Command]++
	}
	t.Logf("commands recorded: %v", got)
	if got["mcp__db__query"] != 2 || got["mcp__mail__search"] != 1 {
		t.Fatalf("commands recorded: %v", got)
	}
}

// HTML source read through a pipe is code, not a page, for the agents that
// optimize after the run too.
func TestResultHooksKeepHTMLSource(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "l.db"))
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	html := "<!doctype html>\n<html><head><title>Docs</title></head><body>\n" +
		strings.Repeat("<div class=\"row\"><p>some paragraph text here</p></div>\n", 200) + "</body></html>\n"
	resp, _ := json.Marshal(map[string]any{"output": html, "exit_code": 0})
	gem, _ := json.Marshal(map[string]any{"llmContent": html, "returnDisplay": "x"})
	tests := []struct {
		agent string
		in    hookInput
	}{
		{"codex", hookInput{HookEventName: "PostToolUse", ToolName: "Bash", ToolResponse: resp}},
		{"gemini", hookInput{HookEventName: "AfterTool", ToolName: "run_shell_command", ToolResponse: gem}},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			tt.in.ToolInput = map[string]any{"command": "sed -n 1,200p page.html | cut -c1-80"}
			reply, err := agents[tt.agent].hook(t.Context(), &common{attrs: attrFlag{}}, tt.in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			if reply == nil {
				return // left whole
			}
			m, _ := reply.(map[string]any)
			if got, _ := m["reason"].(string); !strings.Contains(got, "<div") {
				t.Fatalf("source lost: %.200s", got)
			}
		})
	}
}

// An MCP fetch call's url and query reach the pipeline in the hook, as in the
// LLM proxy; without them the page was not typed web and never sampled for the query.
func TestHookedMCPCallPassesArgs(t *testing.T) {
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "l.db"))
	cfg := filepath.Join(t.TempDir(), "c.yaml")
	os.WriteFile(cfg, []byte("{}\n"), 0o644)
	t.Setenv("GASCOIGNE_CONFIG", cfg)
	data, err := os.ReadFile("../../testdata/eval/web/browser-snapshot.md")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := json.Marshal(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(data)}}})
	run := func(input map[string]any) string {
		in := hookInput{HookEventName: "PostToolUse", ToolName: "mcp__web__fetch", ToolInput: input, ToolResponse: resp}
		reply, err := agents["claude"].hook(t.Context(), &common{attrs: attrFlag{}}, in, "gascoigne exec")
		if err != nil || reply == nil {
			t.Fatalf("reply %v, err %v", reply, err)
		}
		out := reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["updatedToolOutput"].(map[string]any)
		return out["content"].([]any)[0].(map[string]any)["text"].(string)
	}
	bare := run(nil)
	withArgs := run(map[string]any{"url": "https://example.com/shop", "query": "battery fusion"})
	if n := strings.Count(strings.ToLower(withArgs), "fusion"); n <= strings.Count(strings.ToLower(bare), "fusion") {
		t.Errorf("query kept no more matching lines: %d with args, %d without", n, strings.Count(strings.ToLower(bare), "fusion"))
	}
}
