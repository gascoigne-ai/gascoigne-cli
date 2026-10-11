package httpproxyreceiver

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lukaszb/gascoigne/component"
)

// usageLedger records usage only.
type usageLedger struct {
	component.Ledger
	mu  sync.Mutex
	got []component.Usage
}

func (l *usageLedger) RecordUsage(_ context.Context, u component.Usage) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.got = append(l.got, u)
	return nil
}

// metering starts a proxy whose upstream answers every request with body.
func metering(t *testing.T, contentType, body string) (string, *usageLedger) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		for _, part := range strings.SplitAfter(body, "\n") { // arrive in pieces
			io.WriteString(w, part)
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(upstream.Close)
	cfg := NewFactory().DefaultConfig().(*Config)
	cfg.Anthropic, cfg.OpenAI, cfg.Gemini = upstream.URL, upstream.URL, upstream.URL
	r := New(cfg, &dropComments{}, nil)
	l := &usageLedger{}
	r.usage = l
	proxy := httptest.NewServer(r)
	t.Cleanup(proxy.Close)
	return proxy.URL, l
}

func TestUsageFromResponses(t *testing.T) {
	tests := []struct {
		name, path, contentType, body string
		hdr                           map[string]string
		want                          component.Usage
	}{
		{
			"anthropic stream", "/v1/messages", "text/event-stream",
			"event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-opus-5-5\",\"usage\":{\"input_tokens\":12,\"cache_read_input_tokens\":9000,\"cache_creation_input_tokens\":300,\"output_tokens\":1}}}\n\n" +
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"text\":\"usage is fine\"}}\n\n" +
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":240}}\n\n",
			map[string]string{"anthropic-version": "2023-06-01"},
			component.Usage{Agent: "claude", Model: "claude-opus-5-5", Input: 12, CacheRead: 9000, CacheWrite: 300, Output: 240},
		},
		{
			"anthropic json", "/v1/messages", "application/json",
			`{"model":"claude-sonnet-5-5","usage":{"input_tokens":40,"cache_read_input_tokens":0,"cache_creation_input_tokens":0,"output_tokens":7}}`,
			map[string]string{"anthropic-version": "2023-06-01"},
			component.Usage{Agent: "claude", Model: "claude-sonnet-5-5", Input: 40, Output: 7},
		},
		{
			"chat completions stream", "/v1/chat/completions", "text/event-stream",
			"data: {\"model\":\"gpt-5.5\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: {\"model\":\"gpt-5.5\",\"choices\":[],\"usage\":{\"prompt_tokens\":5000,\"completion_tokens\":90,\"prompt_tokens_details\":{\"cached_tokens\":4096}}}\n\ndata: [DONE]\n\n",
			nil,
			component.Usage{Agent: "claude", Model: "gpt-5.5", Input: 904, CacheRead: 4096, Output: 90},
		},
		{
			"responses stream", "/v1/responses", "text/event-stream",
			"event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"model\":\"gpt-5.3-codex\"}}\n\n" +
				"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"model\":\"gpt-5.3-codex\",\"usage\":{\"input_tokens\":7000,\"input_tokens_details\":{\"cached_tokens\":6000},\"output_tokens\":300}}}",
			nil,
			component.Usage{Agent: "claude", Model: "gpt-5.3-codex", Input: 1000, CacheRead: 6000, Output: 300},
		},
		{
			"gemini stream", "/v1beta/models/gemini-3.1-pro:streamGenerateContent", "text/event-stream",
			"data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":3000,\"candidatesTokenCount\":10},\"modelVersion\":\"gemini-3.1-pro\"}\n\n" +
				"data: {\"candidates\":[],\"usageMetadata\":{\"promptTokenCount\":3000,\"cachedContentTokenCount\":2000,\"candidatesTokenCount\":80,\"thoughtsTokenCount\":20},\"modelVersion\":\"gemini-3.1-pro\"}\n\n",
			nil,
			component.Usage{Agent: "claude", Model: "gemini-3.1-pro", Input: 1000, CacheRead: 2000, Output: 100},
		},
		{
			"gemini json array", "/v1beta/models/gemini-3.1-pro:streamGenerateContent", "application/json",
			`[{"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":5},"modelVersion":"gemini-3.1-pro"},{"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":9},"modelVersion":"gemini-3.1-pro"}]`,
			nil,
			component.Usage{Agent: "claude", Model: "gemini-3.1-pro", Input: 50, Output: 9},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, l := metering(t, tt.contentType, tt.body)
			req, _ := http.NewRequest("POST", url+tt.path, bytes.NewReader([]byte(`{}`)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "claude-cli/2.1")
			for k, v := range tt.hdr {
				req.Header.Set(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(got) != tt.body {
				t.Fatalf("body changed:\n%q", got)
			}
			l.mu.Lock()
			defer l.mu.Unlock()
			if len(l.got) != 1 {
				t.Fatalf("recorded %d usages", len(l.got))
			}
			u := l.got[0]
			u.TS = tt.want.TS
			if u != tt.want {
				t.Fatalf("got %+v\nwant %+v", u, tt.want)
			}
		})
	}
}

func TestUsageNotRecordedForErrors(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, `{"usage":{"input_tokens":1}}`)
	}))
	t.Cleanup(upstream.Close)
	cfg := NewFactory().DefaultConfig().(*Config)
	cfg.Anthropic = upstream.URL
	r := New(cfg, &dropComments{}, nil)
	l := &usageLedger{}
	r.usage = l
	proxy := httptest.NewServer(r)
	t.Cleanup(proxy.Close)
	post(t, proxy.URL, []byte(`{}`), nil)
	if len(l.got) != 0 {
		t.Fatalf("recorded usage of an error: %+v", l.got)
	}
}

func TestGeminiResults(t *testing.T) {
	_, fake, proxy, got, _ := setup(t)
	src := "package a\n// one\n// two\nfunc A() {}\n"
	body, _ := json.Marshal(map[string]any{"contents": []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "read both"}}},
		map[string]any{"role": "model", "parts": []any{
			map[string]any{"functionCall": map[string]any{"name": "read_file", "args": map[string]any{"absolute_path": "/src/a.go"}}},
			map[string]any{"functionCall": map[string]any{"name": "run_shell_command", "args": map[string]any{"command": "cat b.go"}}},
		}},
		map[string]any{"role": "user", "parts": []any{
			map[string]any{"functionResponse": map[string]any{"name": "read_file", "response": map[string]any{"output": src}}},
			map[string]any{"functionResponse": map[string]any{"name": "run_shell_command", "response": map[string]any{"output": "no comments here"}}},
		}},
	}})
	req, _ := http.NewRequest("POST", proxy.URL+"/v1beta/models/gemini-3.1-pro:streamGenerateContent?alt=sse", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", "k")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	var doc struct {
		Contents []struct {
			Parts []struct {
				FunctionResponse *struct {
					Response map[string]string `json:"response"`
				} `json:"functionResponse"`
			} `json:"parts"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(*got, &doc); err != nil {
		t.Fatal(err)
	}
	parts := doc.Contents[2].Parts
	if out := parts[0].FunctionResponse.Response["output"]; !strings.HasPrefix(out, "// … comments\npackage a\nfunc A() {}\n[gascoigne: optimized") {
		t.Fatalf("read_file result not shortened:\n%s", out)
	}
	if out := parts[1].FunctionResponse.Response["output"]; out != "no comments here" {
		t.Fatalf("shell result changed: %q", out)
	}
	if fake.calls.Load() != 2 {
		t.Fatalf("pipeline calls = %d", fake.calls.Load())
	}
}

func TestUsageFromCompressedStream(t *testing.T) {
	sse := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"model\":\"claude-opus-5-5\",\"usage\":{\"input_tokens\":3,\"cache_read_input_tokens\":500,\"cache_creation_input_tokens\":20,\"output_tokens\":1}}}\n\n" +
		"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":42}}\n\n"
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte(sse))
	zw.Close()
	tests := []struct {
		encoding string
		body     []byte
		metered  bool
	}{
		{"gzip", gz.Bytes(), true},
		{"br", []byte("not really brotli"), false},
	}
	for _, tt := range tests {
		t.Run(tt.encoding, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Content-Encoding", tt.encoding)
				for b := tt.body; len(b) > 0; b = b[min(7, len(b)):] { // odd-sized pieces
					w.Write(b[:min(7, len(b))])
					w.(http.Flusher).Flush()
				}
			}))
			t.Cleanup(upstream.Close)
			cfg := NewFactory().DefaultConfig().(*Config)
			cfg.Anthropic = upstream.URL
			r := New(cfg, &dropComments{}, nil)
			l := &usageLedger{}
			r.usage = l
			proxy := httptest.NewServer(r)
			t.Cleanup(proxy.Close)

			req, _ := http.NewRequest("POST", proxy.URL+"/v1/messages", strings.NewReader(`{}`))
			req.Header.Set("Accept-Encoding", tt.encoding) // set by hand: the client keeps the bytes as sent
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if !bytes.Equal(got, tt.body) {
				t.Fatalf("body changed: %q", got)
			}
			l.mu.Lock()
			defer l.mu.Unlock()
			if tt.metered != (len(l.got) == 1) {
				t.Fatalf("recorded %+v", l.got)
			}
			if tt.metered && (l.got[0].CacheRead != 500 || l.got[0].Output != 42) {
				t.Fatalf("usage %+v", l.got[0])
			}
		})
	}
}
