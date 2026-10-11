package mcpproxyreceiver

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

type pipeline struct{}

func (pipeline) Process(_ context.Context, in *payload.Payload) (*payload.Payload, error) {
	out := in.Clone()
	out.Blocks[0].Text = "short:" + in.Attrs[payload.AttrTool]
	out.Set(payload.AttrRawID, "id1")
	out.Set(payload.AttrRawTokens, "100")
	out.Set(payload.AttrOutTokens, "2")
	return out, nil
}

type ledger struct{ component.Ledger }

func (ledger) GetRaw(context.Context, string) (string, *payload.Payload, error) {
	return "", payload.New(payload.ToolResult, strings.Repeat("big ", 100)), nil
}

type SearchInput struct {
	Q string `json:"q"`
}

func TestProxy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Downstream server with one tool returning a big result.
	down := mcp.NewServer(&mcp.Implementation{Name: "search", Version: "1"}, nil)
	mcp.AddTool(down, &mcp.Tool{Name: "search", Description: "search"},
		func(_ context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Repeat(in.Q+" ", 100)}}}, nil, nil
		})
	downServerT, downClientT := mcp.NewInMemoryTransports()
	go down.Run(ctx, downServerT)

	upServerT, upClientT := mcp.NewInMemoryTransports()
	r := &Receiver{
		cfg:      &Config{Retrieve: "gascoigne_retrieve"},
		next:     pipeline{},
		log:      slog.New(slog.DiscardHandler),
		ledger:   ledger{},
		upstream: upServerT,
	}
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
	if strings.Join(names, ",") != "gascoigne_retrieve,search" {
		t.Fatalf("tools %v", names)
	}
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"q": "go"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; got != "short:search\n[optimized 100→2 tokens · full: gascoigne_retrieve id=id1]" {
		t.Fatalf("result %q", got)
	}
	res, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "gascoigne_retrieve", Arguments: map[string]any{"id": "id1"}})
	if err != nil || !strings.HasPrefix(res.Content[0].(*mcp.TextContent).Text, "big big") {
		t.Fatalf("retrieve = %+v, %v", res, err)
	}
}

func TestNoDownstreamIsNoop(t *testing.T) {
	r := &Receiver{cfg: &Config{Retrieve: "x"}}
	if err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCompactSchema(t *testing.T) {
	in := json.RawMessage(`{"title":"searchArguments","type":"object","properties":{"q":{"title":"Q","type":"string"}}}`)
	got, _ := json.Marshal(compactSchema("search", in))
	if want := `{"properties":{"q":{"type":"string"}},"type":"object"}`; string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	plain := map[string]any{"type": "object"}
	if out := compactSchema("x", plain); out.(map[string]any)["type"] != "object" {
		t.Fatalf("unchanged schema: %v", out)
	}
}
