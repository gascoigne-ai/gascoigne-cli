package mcpproxyreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// statLedger records what the proxy reports about tools.
type statLedger struct {
	component.Ledger
	mu     sync.Mutex
	agent  string
	server string
	sizes  map[string]int
	calls  map[string]int
	listed chan struct{}
}

func (s *statLedger) RecordToolList(_ context.Context, agent, server string, sizes map[string]int) error {
	s.mu.Lock()
	s.agent, s.server, s.sizes = agent, server, sizes
	s.mu.Unlock()
	close(s.listed)
	return nil
}

func (s *statLedger) RecordToolCall(_ context.Context, _, _, tool string) error {
	s.mu.Lock()
	s.calls[tool]++
	s.mu.Unlock()
	return nil
}

type echoInput struct {
	Text string `json:"text,omitempty" jsonschema:"text to echo"`
}

// fixtureServer has n tools with titled schemas, the shape of a real server
// whose definitions are re-sent with every request.
func fixtureServer(n int) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "fixture-server", Version: "1"}, nil)
	for i := range n {
		mcp.AddTool(srv, &mcp.Tool{
			Name:        fmt.Sprintf("tool_%02d", i),
			Description: fmt.Sprintf("Does thing number %d for a repository, returning a page of results.", i),
		}, func(_ context.Context, _ *mcp.CallToolRequest, in echoInput) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: in.Text}}}, nil, nil
		})
	}
	return srv
}

// TestToolStatsRecorded is the eval for the stats the weekly MCP review is
// built on: the sizes the ledger gets are the sizes of the definitions the
// agent receives, and every call is counted once.
func TestToolStatsRecorded(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	down := fixtureServer(30)
	downServerT, downClientT := mcp.NewInMemoryTransports()
	go down.Run(ctx, downServerT)

	led := &statLedger{calls: map[string]int{}, listed: make(chan struct{})}
	upServerT, upClientT := mcp.NewInMemoryTransports()
	r := &Receiver{
		cfg:      &Config{Retrieve: "gascoigne_retrieve", CompactTools: true},
		next:     pipeline{},
		log:      slog.New(slog.DiscardHandler),
		ledger:   led,
		upstream: upServerT,
	}
	go r.Run(WithTransport(ctx, downClientT))

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, upClientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()

	seen := map[string]int{} // tokens of each definition as the agent receives it
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(tool)
		seen[tool.Name] = tokens.Estimate(string(b))
	}
	select {
	case <-led.listed:
	case <-time.After(5 * time.Second):
		t.Fatal("tool list was never recorded")
	}

	led.mu.Lock()
	agent, server, sizes := led.agent, led.server, led.sizes
	led.mu.Unlock()
	if agent != "agent" || server != "fixture-server" {
		t.Fatalf("recorded for agent %q, server %q", agent, server)
	}
	if len(sizes) != 30 {
		t.Fatalf("recorded %d tools, want 30", len(sizes))
	}
	for i := range 30 {
		name := fmt.Sprintf("tool_%02d", i)
		if sizes[name] == 0 || sizes[name] != seen[name] {
			t.Errorf("%s: recorded %d tokens, the agent receives %d", name, sizes[name], seen[name])
		}
	}

	for _, name := range []string{"tool_03", "tool_03", "tool_17"} {
		if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"text": "hi"}}); err != nil {
			t.Fatal(err)
		}
	}
	led.mu.Lock()
	defer led.mu.Unlock()
	if led.calls["tool_03"] != 2 || led.calls["tool_17"] != 1 || len(led.calls) != 2 {
		t.Fatalf("calls %v", led.calls)
	}
}

// TestNoStatsLedgerIsFine: a ledger without the optional interface changes nothing.
func TestNoStatsLedgerIsFine(t *testing.T) {
	r := &Receiver{ledger: ledger{}, log: slog.New(slog.DiscardHandler)}
	r.recordCall(t.Context(), nil, "x") // would panic on a nil session if it tried to record
	r.recordList(t.Context(), nil, map[string]int{"x": 1})
}
