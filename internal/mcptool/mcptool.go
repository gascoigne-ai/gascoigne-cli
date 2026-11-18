// Package mcptool holds MCP pieces shared by MCP receivers: the retrieve and
// stats tools and the hint appended to optimized results.
package mcptool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/internal/hint"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// LedgerID is the id of the one ledger extension.
const LedgerID = "ledger"

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
func Text(body *payload.Payload) string {
	text := body.Text()
	id := body.Attrs[payload.AttrRawID]
	if id == "" {
		return text
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + hint.Of(id)
}

// Result wraps text as a tool result.
func Result(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// RetrieveInput is the argument of the retrieve tool, which returns an original or
// parts of it.
type RetrieveInput struct {
	ID      string `json:"id" jsonschema:"id from an [optimized ... id=<id>] hint"`
	Lines   string `json:"lines,omitempty" jsonschema:"only these lines, e.g. 120-180"`
	Grep    string `json:"grep,omitempty" jsonschema:"only lines matching this regexp, numbered"`
	Query   string `json:"query,omitempty" jsonschema:"only the lines best matching these words, numbered"`
	Context int    `json:"context,omitempty" jsonschema:"lines of context around grep/query matches"`
}

// AddRetrieve registers a tool returning original, unoptimized content.
// Asking for all of it marks the source as badly optimized, so next time it is
// not; a part (lines, grep, query) is a lookup, not a complaint.
func AddRetrieve(server *mcp.Server, name string, ledger component.Ledger) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        name,
		Description: "Return the original content of an optimized result, by the id from its [optimized ...] hint; lines, grep or query return only part of it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RetrieveInput) (*mcp.CallToolResult, any, error) {
		sl := Slice{Lines: in.Lines, Grep: in.Grep, Query: in.Query, Context: in.Context}
		text, err := Original(ctx, ledger, in.ID, sl)
		if err != nil {
			return nil, nil, err
		}
		return Result(text), nil, nil
	})
}

// Original returns the original stored under id, or the part sl selects.
// A whole read marks the source as failing: the optimized output was not enough.
func Original(ctx context.Context, ledger component.Ledger, id string, sl Slice) (string, error) {
	key, p, err := ledger.GetRaw(ctx, id)
	if err != nil {
		return "", err
	}
	if key != "" && !sl.Partial() {
		if err := ledger.MarkFailure(ctx, key, component.ReasonRawRequested); err != nil {
			return "", err
		}
	}
	return sl.Apply(p.Text())
}

// StatsInput is the argument of the stats tool.
type StatsInput struct {
	By      string `json:"by,omitempty" jsonschema:"one grouping (command, optimizer, family, outcome, reason, session, route); default: overview"`
	Session string `json:"session,omitempty" jsonschema:"session id; default this session, 'all' for every session"`
	Format  string `json:"format,omitempty" jsonschema:"table (default) or json"`
}

// AddStats registers a tool reporting token savings, by default per optimizer
// family in the calling session.
func AddStats(server *mcp.Server, ledger component.Ledger, sessions *Sessions) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "stats",
		Description: "Token savings so far (this session by default): overview of outcomes, commands, optimizers and skips, or one grouping.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StatsInput) (*mcp.CallToolResult, any, error) {
		q := component.StatsQuery{By: in.By, Session: in.Session}
		switch q.Session {
		case "":
			q.Session = sessions.Of(req)
		case "all":
			q.Session = ""
		}
		if q.By == "" {
			o, err := BuildOverview(ctx, ledger, q)
			if err != nil {
				return nil, nil, err
			}
			if in.Format == "json" {
				data, err := json.Marshal(o)
				return Result(string(data)), nil, err
			}
			return Result(o.Table()), nil, nil
		}
		stats, err := ledger.Stats(ctx, q)
		if err != nil {
			return nil, nil, err
		}
		if in.Format == "json" {
			data, err := json.Marshal(Report(q, stats))
			return Result(string(data)), nil, err
		}
		return Result(statsTable(q.By, stats)), nil, nil
	})
}
