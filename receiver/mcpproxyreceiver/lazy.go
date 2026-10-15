package mcpproxyreceiver

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// searchResults is how many tools search_tools returns with their schemas.
const searchResults = 5

// Names of the two tools that reach lazy tools.
const (
	searchToolName = "search_tools"
	callToolName   = "call_tool"
)

// maxListed is how many tool names the description of search_tools spells out.
const maxListed = 40

// lazyDescription says what search_tools finds. When only some tools are
// lazy it names them (sorted, capped: the same choices give the same bytes,
// which keeps the agent's prompt cache), so the agent knows they exist
// and asks for them explicitly.
func lazyDescription(tools []*mcp.Tool, all bool) string {
	if all {
		return fmt.Sprintf("Find tools of this server (%d tools) by what they do; returns the best matches with their input schemas. Run one with call_tool.", len(tools))
	}
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	slices.Sort(names)
	list := strings.Join(names[:min(len(names), maxListed)], ", ")
	if len(names) > maxListed {
		list += fmt.Sprintf(", and %d more", len(names)-maxListed)
	}
	return fmt.Sprintf("%d more tools of this server are not listed: %s. Find one by what it does; returns the best matches with their input schemas. Run one with call_tool.", len(tools), list)
}

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
func (r *Receiver) addLazy(s *mcp.Server, cs *mcp.ClientSession, tools []*mcp.Tool, all bool) {
	byName := map[string]*mcp.Tool{}
	names := make([]string, len(tools))
	docs := make([][]string, len(tools))
	for i, t := range tools {
		byName[t.Name] = t
		names[i] = t.Name
		docs[i] = sample.Terms(t.Name + " " + strings.ReplaceAll(t.Name, "_", " ") + " " + t.Description)
	}
	mcp.AddTool(s, &mcp.Tool{
		Name:        searchToolName,
		Description: lazyDescription(tools, all),
	}, func(_ context.Context, _ *mcp.CallToolRequest, in SearchToolsInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Query) == "" {
			return mcptool.Result(strings.Join(names, "\n")), nil, nil
		}
		scores := sample.BM25(docs, sample.Terms(in.Query))
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
		Name:        callToolName,
		Description: "Run a tool of this server found with search_tools.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in CallToolInput) (*mcp.CallToolResult, any, error) {
		if byName[in.Name] == nil {
			return nil, nil, fmt.Errorf("unknown tool %q; find one with search_tools", in.Name)
		}
		res, err := r.call(ctx, cs, in.Name, in.Arguments, req)
		return res, nil, err
	})
}
