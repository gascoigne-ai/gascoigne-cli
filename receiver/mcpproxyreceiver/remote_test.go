package mcpproxyreceiver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestRemote(t *testing.T) {
	newDown := func() *mcp.Server {
		down := mcp.NewServer(&mcp.Implementation{Name: "search", Version: "1"}, nil)
		mcp.AddTool(down, &mcp.Tool{Name: "search", Description: "search"},
			func(_ context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat(in.Q+" ", 100)}}}, nil, nil
			})
		return down
	}
	for _, tc := range []struct {
		name    string
		sse     bool
		auto    bool
		handler http.Handler
	}{
		{"streamable", false, false, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return newDown() }, nil)},
		{"sse", true, false, mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return newDown() }, nil)},
		{"auto finds streamable", false, true, mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return newDown() }, nil)},
		{"auto falls back to sse", false, true, mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return newDown() }, nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var auth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				auth = req.Header.Get("Authorization")
				tc.handler.ServeHTTP(w, req)
			}))
			defer srv.Close()

			upServerT, upClientT := mcp.NewInMemoryTransports()
			r := &Receiver{
				cfg:      &Config{Retrieve: "gascoigne_retrieve"},
				next:     pipeline{},
				log:      slog.New(slog.DiscardHandler),
				ledger:   ledger{},
				upstream: upServerT,
			}
			remote := Remote{URL: srv.URL, SSE: tc.sse, Auto: tc.auto, Headers: map[string]string{"Authorization": "Bearer k"}}
			go r.Run(WithRemote(t.Context(), remote))

			cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(t.Context(), upClientT, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"q": "go"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Content[0].(*mcp.TextContent).Text; !strings.HasPrefix(got, "short:search") {
				t.Fatalf("result %q", got)
			}
			if auth != "Bearer k" {
				t.Fatalf("Authorization = %q", auth)
			}
			if strings.Contains(r.server, "127.0.0.1") || !strings.HasPrefix(r.server, "search#") {
				t.Fatalf("server id %q must hash the url", r.server)
			}
		})
	}
}

func TestRemoteValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"https", Config{Retrieve: "r", URL: "https://x.test/mcp"}, true},
		{"not http", Config{Retrieve: "r", URL: "file:///etc/passwd"}, false},
		{"with command", Config{Retrieve: "r", URL: "https://x.test/mcp", Command: []string{"x"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg.Validate(); (err == nil) != tc.ok {
				t.Fatalf("Validate = %v", err)
			}
		})
	}
}
