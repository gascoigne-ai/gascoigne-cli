package httpproxyreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/lukaszb/gascoigne/payload"
)

// dropComments is a fake pipeline: drops "//" lines (as the cat optimizer
// would, with a marker) and counts calls.
type dropComments struct{ calls atomic.Int32 }

func (d *dropComments) Process(_ context.Context, p *payload.Payload) (*payload.Payload, error) {
	d.calls.Add(1)
	var kept []string
	for _, l := range strings.Split(strings.TrimRight(p.Text(), "\n"), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(l), "//") {
			kept = append(kept, l)
		}
	}
	if len(kept) == len(strings.Split(strings.TrimRight(p.Text(), "\n"), "\n")) {
		return p, nil
	}
	out := payload.New(payload.ToolResult, strings.Join(append([]string{"// … comments"}, kept...), "\n")+"\n")
	out.Attrs = map[string]string{payload.AttrRawID: "abc", payload.AttrRawTokens: "100", payload.AttrOutTokens: "20"}
	return out, nil
}

func setup(t *testing.T) (*Receiver, *dropComments, *httptest.Server, *[]byte, *http.Header) {
	t.Helper()
	var got []byte
	var hdr http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		hdr = r.Header.Clone()
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "event: message_start\ndata: {}\n\n")
	}))
	t.Cleanup(upstream.Close)
	cfg := NewFactory().DefaultConfig().(*Config)
	cfg.Anthropic, cfg.OpenAI = upstream.URL, upstream.URL
	fake := &dropComments{}
	r := New(cfg, fake, nil)
	proxy := httptest.NewServer(r)
	t.Cleanup(proxy.Close)
	return r, fake, proxy, &got, &hdr
}

// numbered renders lines the way Claude Code shows a Read result.
func numbered(lines ...string) string {
	var b strings.Builder
	for i, l := range lines {
		b.WriteString(strconv.Itoa(i+1) + "\t" + l + "\n")
	}
	return b.String()
}

func anthropicBody(result string, tool string) []byte {
	b, _ := json.Marshal(map[string]any{
		"model": "claude-opus-5-5", "max_tokens": 64000, "stream": true,
		"system": "keep me",
		"messages": []any{
			map[string]any{"role": "user", "content": "read it"},
			map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "t1", "name": tool, "input": map[string]any{"file_path": "/src/a.go"}}}},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": "t1", "content": result}}},
		},
	})
	return b
}

func post(t *testing.T, url string, body []byte, hdr map[string]string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return string(out)
}

func toolResult(t *testing.T, body []byte) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("forwarded body: %v", err)
	}
	msgs := doc["messages"].([]any)
	last := msgs[len(msgs)-1].(map[string]any)["content"].([]any)[0].(map[string]any)
	return last["content"].(string)
}

func TestReadKeepsLineNumbers(t *testing.T) {
	_, fake, proxy, got, hdr := setup(t)
	src := numbered("package a", "", "// Add sums.", "// More words.", "func Add() {}", "", "// tail")
	resp := post(t, proxy.URL, anthropicBody(src, "Read"), map[string]string{"anthropic-beta": "oauth-2025-04-20", "x-api-key": "k"})
	if !strings.Contains(resp, "message_start") {
		t.Fatalf("response not streamed through: %q", resp)
	}
	res := toolResult(t, *got)
	want := "// … comments\n1\tpackage a\n2\t\n5\tfunc Add() {}\n[gascoigne: optimized 100→20 tokens · full: gascoigne raw abc]"
	if res != want {
		t.Fatalf("got\n%q\nwant\n%q", res, want)
	}
	if hdr.Get("anthropic-beta") != "oauth-2025-04-20" || hdr.Get("x-api-key") != "k" {
		t.Fatalf("headers not forwarded: %v", hdr)
	}
	var doc map[string]any
	json.Unmarshal(*got, &doc)
	if doc["system"] != "keep me" || doc["max_tokens"] != float64(64000) {
		t.Fatalf("rest of the body changed: %v", doc)
	}
	// The same conversation again: identical bytes, no second optimization.
	first := string(*got)
	post(t, proxy.URL, anthropicBody(src, "Read"), nil)
	if string(*got) != first || fake.calls.Load() != 1 {
		t.Fatalf("not stable: calls %d", fake.calls.Load())
	}
}

func TestLeftAlone(t *testing.T) {
	src := numbered("a", "// b", "c", "// d", "e", "f")
	tests := []struct {
		name, tool, result string
		hdr                map[string]string
	}{
		{"edit tools", "Edit", src, nil},
		{"already optimized", "Bash", "x\n// y\n[gascoigne: optimized 10→2 tokens · full: gascoigne raw ab]", nil},
		{"raw requested", "Read", src, map[string]string{RawHeader: "1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, proxy, got, hdr := setup(t)
			post(t, proxy.URL, anthropicBody(tt.result, tt.tool), tt.hdr)
			if res := toolResult(t, *got); res != tt.result {
				t.Fatalf("changed: %q", res)
			}
			if hdr.Get(RawHeader) != "" {
				t.Fatal("raw header forwarded")
			}
		})
	}
}

func TestResponsesShellOutput(t *testing.T) {
	_, fake, proxy, got, _ := setup(t)
	body, _ := json.Marshal(map[string]any{"model": "gpt-5.3-codex", "input": []any{
		map[string]any{"type": "function_call", "call_id": "c1", "name": "shell", "arguments": `{"command":["bash","-lc","cat a.go"]}`},
		map[string]any{"type": "function_call_output", "call_id": "c1", "output": "a\n// b\nc\n"},
	}})
	req, _ := http.NewRequest("POST", proxy.URL+"/v1/responses", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !strings.Contains(string(*got), "gascoigne raw abc") || fake.calls.Load() != 1 {
		t.Fatalf("not optimized: %s", *got)
	}
}

func TestAttrsFromCalls(t *testing.T) {
	tests := []struct {
		call call
		want map[string]string
	}{
		{call{"shell", arguments(`{"command":["bash","-lc","go test ./..."]}`)}, map[string]string{"cmd": "go test ./..."}},
		{call{"exec_command", arguments(`{"cmd":"git status"}`)}, map[string]string{"cmd": "git status"}},
		{call{"Read", map[string]any{"file_path": "/x/a.go"}}, map[string]string{"file": "/x/a.go", "cmd": "cat /x/a.go"}},
		{call{"Read", map[string]any{"file_path": "/x/a.go", "offset": json.Number("120"), "limit": json.Number("40")}},
			map[string]string{"cmd": "sed -n 120,$p /x/a.go"}},
		{call{"Grep", map[string]any{"pattern": "func main", "path": "cmd", "output_mode": "content"}}, map[string]string{"cmd": "rg -n 'func main' cmd"}},
		{call{"Grep", map[string]any{"pattern": "TODO", "output_mode": "files_with_matches"}}, map[string]string{"cmd": "rg -l TODO"}},
		{call{"Glob", map[string]any{"pattern": "**/*.go"}}, map[string]string{"cmd": "find . -name '**/*.go'"}},
		{call{"WebFetch", map[string]any{"url": "https://go.dev/doc", "prompt": "release notes"}}, map[string]string{"url": "https://go.dev/doc", "query": "release notes"}},
		{call{"WebSearch", map[string]any{"query": "go 1.27 iterators"}}, map[string]string{"query": "go 1.27 iterators"}},
		{call{"mcp__db__query", map[string]any{"sql": "select 1"}}, map[string]string{"tool": "mcp__db__query"}},
	}
	for _, tt := range tests {
		got := attrsOf(tt.call)
		for k, v := range tt.want {
			if got[k] != v {
				t.Errorf("%s %v: %s = %q, want %q", tt.call.name, tt.call.input, k, got[k], v)
			}
		}
	}
}
