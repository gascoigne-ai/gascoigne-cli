package mcpproxyreceiver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestLazyTools(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	down := mcp.NewServer(&mcp.Implementation{Name: "big", Version: "1"}, nil)
	for i := range 50 {
		mcp.AddTool(down, &mcp.Tool{Name: fmt.Sprintf("tool_%d", i), Description: fmt.Sprintf("does thing %d", i)},
			func(context.Context, *mcp.CallToolRequest, SearchInput) (*mcp.CallToolResult, any, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ran"}}}, nil, nil
			})
	}
	mcp.AddTool(down, &mcp.Tool{Name: "create_issue", Description: "Open a new GitHub issue in a repository"},
		func(_ context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "issue " + in.Q}}}, nil, nil
		})
	downServerT, downClientT := mcp.NewInMemoryTransports()
	go down.Run(ctx, downServerT)

	upServerT, upClientT := mcp.NewInMemoryTransports()
	r := &Receiver{cfg: &Config{Retrieve: "gascoigne_retrieve", MaxTools: 20}, next: pipeline{},
		log: slog.New(slog.DiscardHandler), ledger: ledger{}, upstream: upServerT}
	go r.Run(WithTransport(ctx, downClientT))

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, upClientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	var names []string
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, ","); got != "call_tool,gascoigne_retrieve,search_tools" {
		t.Fatalf("tools %s", got)
	}
	text := func(res *mcp.CallToolResult) string { return res.Content[0].(*mcp.TextContent).Text }
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search_tools", Arguments: map[string]any{"query": "open an issue"}})
	if err != nil || !strings.HasPrefix(text(res), `[{"description":"Open a new GitHub issue`) {
		t.Fatalf("search: %v %q", err, text(res))
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "create_issue", "arguments": map[string]any{"q": "bug"}}})
	if err != nil || !strings.HasPrefix(text(res), "short:create_issue") {
		t.Fatalf("call: %v %q", err, text(res))
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "nope"}})
	if err == nil && !res.IsError {
		t.Fatal("unknown tool: want error")
	}
}
