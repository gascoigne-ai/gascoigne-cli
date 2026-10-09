// Package mcptool holds MCP pieces shared by MCP receivers: the retrieve and
// stats tools and the hint appended to optimized results.
package mcptool

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

// Ledger looks up the ledger extension by id.
func Ledger(host component.Host, id string) (component.Ledger, error) {
	pid, err := component.ParseID(id)
	if err != nil {
		return nil, err
	}
	ext, ok := host.Extension(pid)
	if !ok {
		return nil, fmt.Errorf("extension %s not configured", pid)
	}
	l, ok := ext.(component.Ledger)
	if !ok {
		return nil, fmt.Errorf("extension %s is not a ledger", pid)
	}
	return l, nil
}

// Text returns the payload text plus, when optimized, how to get the original.
func Text(p *payload.Payload, retrieveTool string) string {
	text := p.Text()
	id := p.Attrs[payload.AttrRawID]
	if id == "" {
		return text
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return fmt.Sprintf("%s[optimized %s→%s tokens · full: %s id=%s]", text,
		p.Attrs[payload.AttrRawTokens], p.Attrs[payload.AttrOutTokens], retrieveTool, id)
}

func Result(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

type RetrieveInput struct {
	ID string `json:"id" jsonschema:"id from an [optimized ... id=<id>] hint"`
}

// AddRetrieve registers a tool returning original, unoptimized content.
// Asking for it marks the source as badly optimized, so next time it is not.
func AddRetrieve(s *mcp.Server, name string, l component.Ledger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: "Return the full original content of an optimized result, by the id from its [optimized ...] hint.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RetrieveInput) (*mcp.CallToolResult, any, error) {
		key, p, err := l.GetRaw(ctx, in.ID)
		if err != nil {
			return nil, nil, err
		}
		if key != "" {
			if err := l.MarkFailure(ctx, key, component.ReasonRawRequested); err != nil {
				return nil, nil, err
			}
		}
		return Result(p.Text()), nil, nil
	})
}

// AddStats registers a tool reporting token savings per route.
func AddStats(s *mcp.Server, l component.Ledger) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stats",
		Description: "Token savings so far, per route.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, any, error) {
		stats, err := l.Stats(ctx)
		if err != nil {
			return nil, nil, err
		}
		var b strings.Builder
		w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "route\truns\traw tok\tout tok\tsaved")
		for _, st := range stats {
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\n", st.Route, st.Runs, st.RawTokens, st.OutTokens, st.Saved())
		}
		w.Flush()
		return Result(b.String()), nil, nil
	})
}
