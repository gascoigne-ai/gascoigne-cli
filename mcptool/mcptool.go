// Package mcptool holds MCP pieces shared by MCP receivers: the retrieve and
// stats tools and the hint appended to optimized results.
package mcptool

import (
	"cmp"
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"text/tabwriter"
	"time"

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

type StatsInput struct {
	By      string `json:"by,omitempty" jsonschema:"route, session, family or optimizer; default family"`
	Session string `json:"session,omitempty" jsonschema:"session id; default this session, 'all' for every session"`
}

// AddStats registers a tool reporting token savings, by default per optimizer
// family in the calling session.
func AddStats(s *mcp.Server, l component.Ledger, sessions *Sessions) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stats",
		Description: "Token savings so far: by family (default), optimizer, route or session; this session by default.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StatsInput) (*mcp.CallToolResult, any, error) {
		q := component.StatsQuery{By: cmp.Or(in.By, component.ByFamily), Session: in.Session}
		switch q.Session {
		case "":
			q.Session = sessions.Of(req)
		case "all":
			q.Session = ""
		}
		stats, err := l.Stats(ctx, q)
		if err != nil {
			return nil, nil, err
		}
		return Result(StatsTable(q.By, stats)), nil, nil
	})
}

type sessionKey struct{}

// WithSession sets an explicit session id (e.g. from the CLI) for MCP servers run with ctx.
func WithSession(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionKey{}, id)
}

// Sessions resolves the session of tool calls: explicit id from the context,
// else the MCP transport's session id, else one id generated per server run.
type Sessions struct{ explicit, generated string }

func NewSessions(ctx context.Context) *Sessions {
	explicit, _ := ctx.Value(sessionKey{}).(string)
	return &Sessions{explicit: explicit, generated: fmt.Sprintf("mcp-%s-%04x", time.Now().Format("20060102-150405"), rand.N(1<<16))}
}

func (s *Sessions) Of(req *mcp.CallToolRequest) string {
	if s.explicit != "" {
		return s.explicit
	}
	if req != nil && req.Session != nil && req.Session.ID() != "" {
		return req.Session.ID()
	}
	return s.generated
}

// StatsTable renders stats with a total row.
func StatsTable(by string, stats []component.Stat) string {
	by = cmp.Or(by, component.ByRoute)
	steps := by == component.ByFamily || by == component.ByOptimizer
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	switch {
	case steps:
		fmt.Fprintf(w, "%s\tsteps\traw tok\tout tok\tsaved\tsaved %%\n", by)
	case by == component.BySession:
		fmt.Fprintf(w, "%s\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %%\tfirst\tlast\n", by)
	default:
		fmt.Fprintf(w, "%s\truns\toptimized\tfailed\traw tok\tout tok\tsaved\tsaved %%\n", by)
	}
	total := component.Stat{Group: "total"}
	row := func(s component.Stat) {
		saved := fmt.Sprintf("%d\t%d\t%d\t%s", s.RawTokens, s.OutTokens, s.Saved(), Percent(s.Saved(), s.RawTokens))
		switch {
		case steps:
			fmt.Fprintf(w, "%s\t%d\t%s\n", s.Group, s.Runs, saved)
		case by == component.BySession && s.Group != "total":
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\t%s\n", s.Group, s.Runs, s.Optimized, s.Failed, saved,
				s.First.Format(time.DateTime), s.Last.Format(time.DateTime))
		default:
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\n", s.Group, s.Runs, s.Optimized, s.Failed, saved)
		}
	}
	for _, s := range stats {
		row(s)
		total.Runs += s.Runs
		total.Optimized += s.Optimized
		total.Failed += s.Failed
		total.RawTokens += s.RawTokens
		total.OutTokens += s.OutTokens
	}
	row(total)
	w.Flush()
	return b.String()
}

// Percent formats part/whole, "-" when whole is 0.
func Percent(part, whole int) string {
	if whole == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(part)/float64(whole))
}
