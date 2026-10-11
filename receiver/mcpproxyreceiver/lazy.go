package mcpproxyreceiver

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/compress/rows"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
)

// searchResults is how many tools search_tools returns with their schemas.
const searchResults = 5

type SearchToolsInput struct {
	Query string `json:"query,omitempty" jsonschema:"what the tool should do; empty lists every tool's name"`
}

type CallToolInput struct {
	Name      string         `json:"name" jsonschema:"tool name from search_tools"`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"arguments matching the tool's input schema"`
}

// addLazy exposes the downstream tools through two tools instead of one
// each: search_tools finds tools and returns their schemas, call_tool runs
// one. Requests then carry two schemas, not hundreds.
func (r *Receiver) addLazy(s *mcp.Server, cs *mcp.ClientSession, tools []*mcp.Tool) {
	byName := map[string]*mcp.Tool{}
	names := make([]string, len(tools))
	docs := make([][]string, len(tools))
	for i, t := range tools {
		byName[t.Name] = t
		names[i] = t.Name
		docs[i] = rows.Terms(t.Name + " " + strings.ReplaceAll(t.Name, "_", " ") + " " + t.Description)
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        "search_tools",
		Description: fmt.Sprintf("Find tools of this server (%d tools) by what they do; returns the best matches with their input schemas. Run one with call_tool.", len(tools)),
	}, func(_ context.Context, _ *mcp.CallToolRequest, in SearchToolsInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Query) == "" {
			return mcptool.Result(strings.Join(names, "\n")), nil, nil
		}
		scores := rows.BM25(docs, rows.Terms(in.Query))
		order := make([]int, len(tools))
		for i := range order {
			order[i] = i
		}
		slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })
		var found []*mcp.Tool
		for _, i := range order[:min(searchResults, len(order))] {
			if scores[i] > 0 {
				found = append(found, tools[i])
			}
		}
		if len(found) == 0 {
			return mcptool.Result("no tool matches; search_tools with an empty query lists them all"), nil, nil
		}
		b, err := json.Marshal(found)
		if err != nil {
			return nil, nil, err
		}
		return mcptool.Result(string(b)), nil, nil
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "call_tool",
		Description: "Run a tool of this server found with search_tools.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CallToolInput) (*mcp.CallToolResult, any, error) {
		if byName[in.Name] == nil {
			return nil, nil, fmt.Errorf("unknown tool %q; find one with search_tools", in.Name)
		}
		res, err := r.call(ctx, cs, in.Name, in.Arguments, req)
		return res, nil, err
	})
}
