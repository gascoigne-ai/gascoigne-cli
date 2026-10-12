package mcpproxyreceiver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/mcpauth"
	"github.com/gascoigne-ai/gascoigne-cli/mcpauth/authtest"
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

func TestRemoteLoginNeeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="http://127.0.0.1/none"`)
		http.Error(w, "no", http.StatusUnauthorized)
	}))
	defer srv.Close()
	for _, tc := range []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{"no login", nil, "gascoigne mcp-login " + srv.URL},
		{"header refused", map[string]string{"authorization": "Bearer x"}, "refused the configured Authorization header"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Receiver{cfg: &Config{Retrieve: "r"}, next: pipeline{}, log: slog.New(slog.DiscardHandler)}
			err := r.Run(WithRemote(t.Context(), Remote{URL: srv.URL + "?key=secret", Headers: tc.headers}))
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRemoteLoginInProxy(t *testing.T) {
	url, _ := authtest.Server(t)
	store := &mcpauth.Store{Path: filepath.Join(t.TempDir(), "mcp-auth.json")}
	open := func(authURL string) {
		go func() { // the "browser": follows the redirect to the login callback
			if resp, err := http.Get(authURL); err == nil {
				resp.Body.Close()
			}
		}()
	}
	upServerT, upClientT := mcp.NewInMemoryTransports()
	r := &Receiver{cfg: &Config{Retrieve: "gascoigne_retrieve"}, next: pipeline{}, log: slog.New(slog.DiscardHandler), ledger: ledger{}, upstream: upServerT}
	go r.Run(WithRemote(t.Context(), Remote{URL: url, Store: store, Open: open}))

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(t.Context(), upClientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	toolNames := func() string {
		var names []string
		for tool, err := range cs.Tools(t.Context(), nil) {
			if err != nil {
				t.Fatal(err)
			}
			names = append(names, tool.Name)
		}
		return strings.Join(names, ",")
	}
	if got := toolNames(); got != "gascoigne_login" || !strings.Contains(cs.InitializeResult().Instructions, "OAuth login") {
		t.Fatalf("before login: tools %q, instructions %q", got, cs.InitializeResult().Instructions)
	}
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "gascoigne_login"})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(got, "/authorize?") {
		t.Fatalf("login result %q", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for toolNames() != "gascoigne_retrieve" {
		if time.Now().After(deadline) {
			t.Fatalf("tools after login: %q", toolNames())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := store.Get(url); !ok {
		t.Fatal("login not saved")
	}
}
