package mcpproxyreceiver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// repoArgs is the argument shape of most tools of a code-hosting server:
// several described parameters, which is what makes real definitions costly.
type repoArgs struct {
	Owner   string `json:"owner" jsonschema:"repository owner, a user or an organization"`
	Repo    string `json:"repo" jsonschema:"repository name without the owner"`
	Query   string `json:"query,omitempty" jsonschema:"search terms, using the host's qualifier syntax such as is:open or label:bug"`
	State   string `json:"state,omitempty" jsonschema:"filter by state: open, closed or all"`
	Page    int    `json:"page,omitempty" jsonschema:"page number of the results, starting at 1"`
	PerPage int    `json:"per_page,omitempty" jsonschema:"results per page, at most 100"`
}

// realServer has n tools shaped like a code-hosting server's. tool_03 fails
// with a protocol error and tool_04 with a tool error, so pass-through can be checked.
func realServer(n int, extra ...*mcp.Tool) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "fixture-server", Version: "1"}, nil)
	for i := range n {
		name := fmt.Sprintf("tool_%02d", i)
		mcp.AddTool(srv, &mcp.Tool{
			Name:        name,
			Description: fmt.Sprintf("Operation %d on a repository: reads or changes one kind of object and returns a page of results with their ids, titles and links.", i),
		}, func(_ context.Context, _ *mcp.CallToolRequest, in repoArgs) (*mcp.CallToolResult, any, error) {
			switch name {
			case "tool_03":
				return nil, nil, errors.New("rate limited")
			case "tool_04":
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "not found: " + in.Repo}}}, nil, nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: name + " " + in.Owner + "/" + in.Repo + " " + in.Query}}}, nil, nil
		})
	}
	for _, t := range extra {
		srv.AddTool(t, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "downstream " + t.Name}}}, nil
		})
	}
	return srv
}

type proxyRun struct {
	cs  *mcp.ClientSession
	led *statLedger
}

func startProxy(t *testing.T, down *mcp.Server, cfg *Config, lazy map[string][]string) proxyRun {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	downServerT, downClientT := mcp.NewInMemoryTransports()
	go down.Run(ctx, downServerT)
	led := &statLedger{calls: map[string]int{}, listed: make(chan struct{})}
	upServerT, upClientT := mcp.NewInMemoryTransports()
	r := &Receiver{cfg: cfg, next: pipeline{}, log: slog.New(slog.DiscardHandler), ledger: led, upstream: upServerT}
	go r.Run(WithLazy(WithTransport(ctx, downClientT), lazy))
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, upClientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return proxyRun{cs, led}
}

func (p proxyRun) tools(t *testing.T) []*mcp.Tool {
	t.Helper()
	var out []*mcp.Tool
	for tool, err := range p.cs.Tools(t.Context(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, tool)
	}
	select {
	case <-p.led.listed:
	case <-time.After(5 * time.Second):
		t.Fatal("tool list was never recorded")
	}
	return out
}

func listTokens(tools []*mcp.Tool) int {
	n := 0
	for _, tool := range tools {
		b, _ := json.Marshal(tool)
		n += tokens.Estimate(string(b))
	}
	return n
}

func names(tools []*mcp.Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}

func lazyNames(from, to int) []string {
	var out []string
	for i := from; i < to; i++ {
		out = append(out, fmt.Sprintf("tool_%02d", i))
	}
	return out
}

var baseCfg = Config{Retrieve: "gascoigne_retrieve", CompactTools: true}

// TestLazyToolsEval measures what loading unused tools on request saves, on a
// server with 30 real-shaped tools of which the agent uses five.
func TestLazyToolsEval(t *testing.T) {
	cfg := baseCfg
	full := startProxy(t, realServer(30), &cfg, nil).tools(t)
	lazyRun := startProxy(t, realServer(30), &cfg, map[string][]string{"fixture-server": lazyNames(5, 30)})
	lazy := lazyRun.tools(t)

	fullTok, lazyTok := listTokens(full), listTokens(lazy)
	t.Logf("tool list: %d tokens listed in full, %d with 25 of 30 tools on request (%.0f%% saved)",
		fullTok, lazyTok, 100*(1-float64(lazyTok)/float64(fullTok)))
	if lazyTok > fullTok*35/100 {
		t.Errorf("lazy list is %d tokens, over 35%% of the full %d", lazyTok, fullTok)
	}

	got := names(lazy)
	slices.Sort(got)
	want := append([]string{"call_tool", "gascoigne_retrieve", "search_tools"}, lazyNames(0, 5)...)
	if !slices.Equal(got, want) {
		t.Fatalf("listed %v, want %v", got, want)
	}
	for _, tool := range lazy {
		if tool.Name == "search_tools" {
			for _, n := range lazyNames(5, 30) {
				if !strings.Contains(tool.Description, n) {
					t.Errorf("search_tools does not name %s: %s", n, tool.Description)
				}
			}
		}
	}

	t.Run("stats say what was shown and what was saved", func(t *testing.T) {
		led := lazyRun.led
		led.mu.Lock()
		defer led.mu.Unlock()
		if len(led.lazy) != 25 || len(led.sizes) != 7 { // five tools and the two lazy-mode tools; our retrieve tool is not the server's
			t.Fatalf("shown %d, lazy %d: %v / %v", len(led.sizes), len(led.lazy), led.sizes, led.lazy)
		}
		if _, ok := led.sizes["gascoigne_retrieve"]; ok {
			t.Error("our retrieve tool is recorded as the server's")
		}
		if led.lazy["tool_10"] == 0 || led.sizes["search_tools"] == 0 {
			t.Errorf("sizes %v lazy %v", led.sizes, led.lazy)
		}
	})
}

// TestLazyListIsStable: the tool list is what the prompt cache keys on, so the
// same choices give the same bytes however they were written.
func TestLazyListIsStable(t *testing.T) {
	cfg := baseCfg
	one := startProxy(t, realServer(30), &cfg, map[string][]string{"fixture-server": lazyNames(5, 30)}).tools(t)
	rev := lazyNames(5, 30)
	slices.Reverse(rev)
	two := startProxy(t, realServer(30), &cfg, map[string][]string{"fixture-server": rev}).tools(t)
	a, _ := json.Marshal(one)
	b, _ := json.Marshal(two)
	if string(a) != string(b) {
		t.Fatalf("tool lists differ:\n%s\n%s", a, b)
	}
}

func TestLazyCallsPassThrough(t *testing.T) {
	cfg := baseCfg
	run := startProxy(t, realServer(30), &cfg, map[string][]string{"fixture-server": lazyNames(5, 30)})
	run.tools(t)
	ctx := t.Context()
	args := map[string]any{"owner": "o", "repo": "r", "query": "q"}

	t.Run("a lazy tool runs through call_tool, counted under its own name", func(t *testing.T) {
		res, err := run.cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "tool_10", "arguments": args}})
		if err != nil || res.IsError {
			t.Fatalf("%+v, %v", res, err)
		}
		if got := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(got, "short:tool_10") {
			t.Fatalf("result %q", got)
		}
		run.led.mu.Lock()
		defer run.led.mu.Unlock()
		if run.led.calls["tool_10"] != 1 || run.led.calls["call_tool"] != 0 {
			t.Fatalf("calls %v", run.led.calls)
		}
	})
	t.Run("a listed tool is still called directly", func(t *testing.T) {
		if res, err := run.cs.CallTool(ctx, &mcp.CallToolParams{Name: "tool_01", Arguments: args}); err != nil || res.IsError {
			t.Fatalf("%+v, %v", res, err)
		}
	})
	t.Run("a tool error comes back as the server sent it", func(t *testing.T) {
		direct := startProxy(t, realServer(30), &cfg, nil)
		want, err := direct.cs.CallTool(ctx, &mcp.CallToolParams{Name: "tool_04", Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		lazyRun := startProxy(t, realServer(30), &cfg, map[string][]string{"fixture-server": {"tool_04"}})
		lazyRun.tools(t)
		got, err := lazyRun.cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "tool_04", "arguments": args}})
		if err != nil {
			t.Fatal(err)
		}
		if got.IsError != want.IsError || got.Content[0].(*mcp.TextContent).Text != want.Content[0].(*mcp.TextContent).Text {
			t.Fatalf("lazy %+v, direct %+v", got, want)
		}
	})
	t.Run("a protocol error is an error either way", func(t *testing.T) {
		res, err := run.cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": "tool_03", "arguments": args}})
		if err == nil && (res == nil || !res.IsError) {
			t.Fatalf("a failing tool returned no error: %+v", res)
		}
	})
	t.Run("an unknown or not-lazy name is refused with a pointer", func(t *testing.T) {
		for _, name := range []string{"nope", "tool_01"} {
			res, err := run.cs.CallTool(ctx, &mcp.CallToolParams{Name: "call_tool", Arguments: map[string]any{"name": name}})
			if err == nil && (res == nil || !res.IsError) {
				t.Fatalf("%s: accepted: %+v", name, res)
			}
		}
	})
	t.Run("search_tools finds a lazy tool by what it does", func(t *testing.T) {
		res, err := run.cs.CallTool(ctx, &mcp.CallToolParams{Name: "search_tools", Arguments: map[string]any{"query": "operation 17"}})
		if err != nil || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, `"tool_17"`) {
			t.Fatalf("%+v, %v", res, err)
		}
	})
}

func TestLazyEdgeCases(t *testing.T) {
	cfg := baseCfg
	t.Run("a server with its own call_tool gets no lazy tools", func(t *testing.T) {
		own := &mcp.Tool{Name: "call_tool", Description: "the server's own", InputSchema: map[string]any{"type": "object"}}
		tools := startProxy(t, realServer(8, own), &cfg, map[string][]string{"fixture-server": lazyNames(2, 8)}).tools(t)
		if len(tools) != 8+1+1 { // eight tools, its own call_tool, our retrieve tool
			t.Fatalf("listed %v", names(tools))
		}
	})
	t.Run("max_tools wins over a choice of tools", func(t *testing.T) {
		c := baseCfg
		c.MaxTools = 5
		tools := startProxy(t, realServer(12), &c, map[string][]string{"fixture-server": lazyNames(0, 3)}).tools(t)
		got := names(tools)
		slices.Sort(got)
		if want := []string{"call_tool", "gascoigne_retrieve", "search_tools"}; !slices.Equal(got, want) {
			t.Fatalf("listed %v", got)
		}
	})
	t.Run("names the server does not have change nothing", func(t *testing.T) {
		tools := startProxy(t, realServer(6), &cfg, map[string][]string{"fixture-server": {"gone", "tool_nope"}}).tools(t)
		if len(tools) != 7 {
			t.Fatalf("listed %v", names(tools))
		}
	})
	t.Run("a choice for another server changes nothing", func(t *testing.T) {
		tools := startProxy(t, realServer(6), &cfg, map[string][]string{"other#12345678": lazyNames(0, 6)}).tools(t)
		if len(tools) != 7 {
			t.Fatalf("listed %v", names(tools))
		}
	})
	t.Run("the names list is capped", func(t *testing.T) {
		tools := startProxy(t, realServer(60), &cfg, map[string][]string{"fixture-server": lazyNames(0, 60)}).tools(t)
		for _, tool := range tools {
			if tool.Name == "search_tools" && !strings.Contains(tool.Description, ", and 20 more") {
				t.Fatalf("not capped: %s", tool.Description)
			}
		}
	})
}
