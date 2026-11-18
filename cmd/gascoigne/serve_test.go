package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/registry"
)

type echoInput struct {
	Q string `json:"q"`
}

// echoServer is a downstream MCP server with one tool, echo.
func echoServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "echo", Version: "1"}, nil)
	mcp.AddTool(srv, &mcp.Tool{Name: "echo", Description: "echo"},
		func(_ context.Context, _ *mcp.CallToolRequest, in echoInput) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo " + in.Q}}}, nil, nil
		})
	return srv
}

// TestHelperEchoServer is not a test: with GASCOIGNE_TEST_ECHO set, the test
// binary runs as a downstream MCP server on stdio, so mcp-proxy has a command to run.
func TestHelperEchoServer(t *testing.T) {
	if os.Getenv("GASCOIGNE_TEST_ECHO") == "" {
		t.Skip("helper process only")
	}
	echoServer().Run(t.Context(), &mcp.StdioTransport{})
	os.Exit(0)
}

// serveConfig is a config with an in-memory ledger, so serving never starts the background server.
func serveConfig(t *testing.T) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte("extensions: { ledger: { path: \":memory:\" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

// serveStdio runs command with args as an MCP server on this process's stdin and
// stdout, and returns a client session connected to it.
func serveStdio(t *testing.T, command func(context.Context, []string) error, args []string) *mcp.ClientSession {
	t.Helper()
	for _, env := range []string{"GASCOIGNE_SESSION", "GASCOIGNE_AGENT", "GASCOIGNE_MODEL", "GASCOIGNE_MCP_LAZY"} {
		t.Setenv(env, "")
	}
	inRead, inWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inRead, outWrite
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- command(ctx, args) }()
	t.Cleanup(func() {
		cancel()
		inWrite.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the server did not stop")
		}
		os.Stdin, os.Stdout = oldIn, oldOut
		inRead.Close()
		outRead.Close()
		outWrite.Close()
	})
	transport := &mcp.IOTransport{Reader: outRead, Writer: inWrite}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func callText(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) string {
	t.Helper()
	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	var text strings.Builder
	for _, c := range res.Content {
		if block, ok := c.(*mcp.TextContent); ok {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

func toolNames(t *testing.T, session *mcp.ClientSession) string {
	t.Helper()
	var names []string
	for tool, err := range session.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tool.Name)
	}
	return strings.Join(names, ",")
}

func TestServeMCPUsesIdentityFlags(t *testing.T) {
	session := serveStdio(t, cmdMCP, []string{"-config", serveConfig(t), "-session", "s9"})
	if got := toolNames(t, session); !strings.Contains(got, "fetch") || !strings.Contains(got, "stats") {
		t.Fatalf("tools %q", got)
	}
	if got := callText(t, session, "stats", nil); !strings.Contains(got, "session s9") {
		t.Fatalf("stats do not name the session: %q", got)
	}
}

func TestServeMCPProxyRemote(t *testing.T) {
	var header string
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return echoServer() }, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {
		header = req.Header.Get("X-Key")
		handler.ServeHTTP(writer, req)
	}))
	t.Cleanup(srv.Close) // runs after the proxy has stopped and closed its session
	session := serveStdio(t, cmdMCPProxy, []string{"-config", serveConfig(t), "-url", srv.URL, "-header", "X-Key: v1"})
	if got := toolNames(t, session); got != "echo,gascoigne_retrieve" && got != "gascoigne_retrieve,echo" {
		t.Fatalf("tools %q", got)
	}
	if got := callText(t, session, "echo", map[string]any{"q": "hi"}); !strings.Contains(got, "echo hi") {
		t.Fatalf("result %q", got)
	}
	if header != "v1" {
		t.Fatalf("X-Key = %q", header)
	}
}

func TestServeMCPProxyCommand(t *testing.T) {
	t.Setenv("GASCOIGNE_TEST_ECHO", "1")
	session := serveStdio(t, cmdMCPProxy, []string{"-config", serveConfig(t), "-session", "s7", "--", os.Args[0], "-test.run=TestHelperEchoServer"})
	if got := callText(t, session, "echo", map[string]any{"q": "cmd"}); !strings.Contains(got, "echo cmd") {
		t.Fatalf("result %q", got)
	}
}

func TestWithFactoriesReplacesBySlot(t *testing.T) {
	base := registry.Factories()
	// The marker family tells the replacement from the built-in factories.
	replacement := component.NewFactory(component.KindReceiver, "mcp",
		func() *struct{} { return &struct{}{} },
		func(context.Context, component.Settings, *struct{}) (component.Component, error) { return nil, nil },
		component.WithFamily("marker"))
	got := withFactories(base, []component.Factory{replacement})
	if len(got) != len(base) {
		t.Fatalf("%d factories, want the %d of the base", len(got), len(base))
	}
	for i, f := range got {
		slot := base[i].Kind() == component.KindReceiver && base[i].Type() == "mcp"
		if (f.Family() == "marker") != slot {
			t.Errorf("factory %d (%s %s): replaced %v, want %v", i, f.Kind(), f.Type(), f.Family() == "marker", slot)
		}
	}
	if extra := withFactories(nil, []component.Factory{replacement}); len(extra) != 1 {
		t.Errorf("a factory without a slot is appended, got %d factories", len(extra))
	}
}
