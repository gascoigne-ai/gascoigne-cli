package httpproxyreceiver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// postJSON sends body to path of the proxy and returns the body the upstream got.
func postJSON(t *testing.T, path string, body map[string]any) (string, int32) {
	t.Helper()
	_, fake, proxy, got, _ := setup(t)
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", proxy.URL+path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return string(*got), fake.calls.Load()
}

func TestChatCompletionsToolOutput(t *testing.T) {
	call := map[string]any{"role": "assistant", "tool_calls": []any{
		map[string]any{"id": "c1", "type": "function", "function": map[string]any{"name": "shell", "arguments": `{"command":["bash","-lc","cat a.go"]}`}},
		"not an object",
	}}
	tests := []struct {
		name     string
		tool     map[string]any
		wantRaw  bool
		wantCall int32
	}{
		{"known call", map[string]any{"role": "tool", "tool_call_id": "c1", "content": "a\n// b\nc\n"}, true, 1},
		{"unknown call id", map[string]any{"role": "tool", "tool_call_id": "zz", "content": "a\n// b\nc\n"}, false, 0},
		{"missing call id", map[string]any{"role": "tool", "content": "a\n// b\nc\n"}, false, 0},
		{"not a tool message", map[string]any{"role": "user", "tool_call_id": "c1", "content": "a\n// b\nc\n"}, false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, calls := postJSON(t, "/v1/chat/completions", map[string]any{"model": "gpt-5", "messages": []any{call, tt.tool}})
			if strings.Contains(got, "[raw:abc]") != tt.wantRaw || calls != tt.wantCall {
				t.Fatalf("raw=%v calls=%d, body %s", strings.Contains(got, "[raw:abc]"), calls, got)
			}
		})
	}
}

func TestResponsesCallKinds(t *testing.T) {
	tests := []struct {
		name  string
		items []any
	}{
		{"custom tool", []any{
			map[string]any{"type": "custom_tool_call", "call_id": "c1", "name": "shell", "input": "cat a.go"},
			map[string]any{"type": "custom_tool_call_output", "call_id": "c1", "output": "a\n// b\nc\n"},
		}},
		{"local shell", []any{
			map[string]any{"type": "local_shell_call", "call_id": "c1", "action": map[string]any{"command": []any{"bash", "-lc", "cat a.go"}}},
			map[string]any{"type": "local_shell_call_output", "call_id": "c1", "output": "a\n// b\nc\n"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, calls := postJSON(t, "/v1/responses", map[string]any{"model": "gpt-5.3-codex", "input": tt.items})
			if !strings.Contains(got, "[raw:abc]") || calls != 1 {
				t.Fatalf("not optimized (%d): %s", calls, got)
			}
		})
	}
	t.Run("output without call", func(t *testing.T) {
		got, calls := postJSON(t, "/v1/responses", map[string]any{"model": "m", "input": []any{
			map[string]any{"type": "function_call_output", "call_id": "x", "output": "a\n// b\nc\n"},
		}})
		if strings.Contains(got, "[raw:abc]") || calls != 0 {
			t.Fatalf("changed: %s", got)
		}
	})
}
