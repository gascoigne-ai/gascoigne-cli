// Package mcpproxyreceiver sits between an agent and another MCP server:
// it mirrors the server's tools and runs their text results through the
// pipeline, adding a retrieve tool for originals.
package mcpproxyreceiver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress/schema"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
	"github.com/gascoigne-ai/gascoigne-cli/version"
)

type Receiver struct {
	cfg      *Config
	next     component.Pipeline
	log      *slog.Logger
	ledger   component.Ledger
	upstream mcp.Transport
	sessions *mcptool.Sessions
	server   string // serverID of the downstream server, the key of its tool stats
}

type ctxKey struct{}

// With sets the downstream server command for this run, overriding config.
func With(ctx context.Context, command []string) context.Context {
	return context.WithValue(ctx, ctxKey{}, command)
}

type lazyKey struct{}

// WithLazy sets which tools each proxied server offers on request only: tool
// names by server id (see serverID). Read when the proxy starts, so a change
// reaches the next session and a session's tool list never changes.
func WithLazy(ctx context.Context, choices map[string][]string) context.Context {
	return context.WithValue(ctx, lazyKey{}, choices)
}

func lazyChoices(ctx context.Context) map[string][]string {
	c, _ := ctx.Value(lazyKey{}).(map[string][]string)
	return c
}

// WithTransport sets the downstream transport directly (tests, in-process servers).
func WithTransport(ctx context.Context, t mcp.Transport) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

func (r *Receiver) Start(_ context.Context, host component.Host) error {
	l, err := mcptool.Ledger(host, r.cfg.Ledger)
	if err != nil {
		return err
	}
	r.ledger = l
	return nil
}

func (r *Receiver) Shutdown(context.Context) error { return nil }

// Run proxies until the agent disconnects; without a downstream it does nothing.
func (r *Receiver) Run(ctx context.Context) error {
	down := r.downstream(ctx)
	if down == nil {
		return nil
	}
	r.sessions = mcptool.NewSessions(ctx)
	client := mcp.NewClient(&mcp.Implementation{Name: "gascoigne-proxy", Version: version.Number}, nil)
	cs, err := client.Connect(ctx, down, nil)
	if err != nil {
		return fmt.Errorf("connect downstream: %w", err)
	}
	defer cs.Close()

	init := cs.InitializeResult()
	info := init.ServerInfo
	if info == nil {
		info = &mcp.Implementation{Name: "gascoigne-proxy", Version: version.Number}
	}
	r.server = serverID(info.Name, r.command(ctx))
	var tools []*mcp.Tool
	for tool, err := range cs.Tools(ctx, nil) {
		if err != nil {
			return fmt.Errorf("list downstream tools: %w", err)
		}
		if tool.Name == r.cfg.Retrieve {
			return fmt.Errorf("downstream tool %q clashes with retrieve tool; set retrieve in config", tool.Name)
		}
		if r.cfg.CompactTools {
			tool.InputSchema = compactSchema(tool.Name, tool.InputSchema)
		}
		tools = append(tools, tool)
	}
	direct, lazy := r.splitLazy(ctx, tools)
	s := mcp.NewServer(info, &mcp.ServerOptions{Instructions: init.Instructions, Logger: r.log})
	var listed sync.Once
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if lr, ok := res.(*mcp.ListToolsResult); ok && err == nil && method == "tools/list" {
				// The agent pays for the definitions only when it lists them, and for the
				// ones it is shown: record exactly those, and what the lazy tools would cost.
				listed.Do(func() { r.recordList(ctx, req.GetSession(), lr.Tools, lazy) })
			}
			return res, err
		}
	})
	switch {
	case r.cfg.MaxTools > 0 && len(tools) > r.cfg.MaxTools: // max_tools hides everything; it wins over a choice of tools
		lazy = tools
		r.addLazy(s, cs, tools, true)
	case len(lazy) > 0:
		for _, tool := range direct {
			s.AddTool(tool, r.forward(cs, tool.Name))
		}
		r.addLazy(s, cs, lazy, false)
	default:
		for _, tool := range tools {
			s.AddTool(tool, r.forward(cs, tool.Name))
		}
	}
	mcptool.AddRetrieve(s, r.cfg.Retrieve, r.ledger)
	return s.Run(ctx, r.upstream)
}

// splitLazy separates the tools the user chose to load on request from the
// rest. Nothing is lazy when the server has a tool named like one of the two
// tools that reach lazy ones.
func (r *Receiver) splitLazy(ctx context.Context, tools []*mcp.Tool) (direct, lazy []*mcp.Tool) {
	chosen := lazyChoices(ctx)[r.server]
	if len(chosen) == 0 {
		return tools, nil
	}
	for _, t := range tools {
		if t.Name == searchToolName || t.Name == callToolName {
			r.log.Warn("server has a tool named like a lazy-mode tool: no tool is lazy", "server", r.server, "tool", t.Name)
			return tools, nil
		}
	}
	for _, t := range tools {
		if slices.Contains(chosen, t.Name) {
			lazy = append(lazy, t)
		} else {
			direct = append(direct, t)
		}
	}
	return direct, lazy
}

// toolSize is what one definition costs in tokens, as the agent receives it.
func toolSize(t *mcp.Tool) int {
	b, err := json.Marshal(t)
	if err != nil {
		return 0
	}
	return tokens.Estimate(string(b))
}

// recordList stores what the agent was shown (tokens by tool name; our own
// retrieve tool is not the server's) and what the lazy tools would have cost.
// Stats are a side record: a failure is logged, never shown to the agent.
func (r *Receiver) recordList(ctx context.Context, session mcp.Session, shown, lazy []*mcp.Tool) {
	rec, ok := r.ledger.(mcptool.ToolStatRecorder)
	if !ok {
		return
	}
	agent := r.sessions.Explicit().Agent
	if ss, ok := session.(*mcp.ServerSession); ok && agent == "" {
		if p := ss.InitializeParams(); p != nil && p.ClientInfo != nil {
			agent = p.ClientInfo.Name
		}
	}
	sizes, hidden := map[string]int{}, map[string]int{}
	for _, t := range shown {
		if t.Name != r.cfg.Retrieve {
			sizes[t.Name] = toolSize(t)
		}
	}
	for _, t := range lazy {
		hidden[t.Name] = toolSize(t)
	}
	if err := rec.RecordToolList(ctx, agent, r.server, sizes, hidden); err != nil {
		r.log.Warn("record tool list", "server", r.server, "err", err)
	}
}

// recordCall counts one call of a downstream tool.
func (r *Receiver) recordCall(ctx context.Context, req *mcp.CallToolRequest, tool string) {
	rec, ok := r.ledger.(mcptool.ToolStatRecorder)
	if !ok {
		return
	}
	if err := rec.RecordToolCall(ctx, r.sessions.Agent(req), r.server, tool); err != nil {
		r.log.Warn("record tool call", "server", r.server, "tool", tool, "err", err)
	}
}

// compactSchema drops what a downstream tool's schema says twice (see
// compress/schema); a schema it cannot read passes as it is.
func compactSchema(name string, in any) any {
	b, err := json.Marshal(in)
	if err != nil {
		return in
	}
	var s map[string]any
	if json.Unmarshal(b, &s) != nil || !schema.Compact(name, s) {
		return in
	}
	return s
}

func (r *Receiver) downstream(ctx context.Context) mcp.Transport {
	if t, ok := ctx.Value(ctxKey{}).(mcp.Transport); ok {
		return t
	}
	if c := r.command(ctx); len(c) > 0 {
		return &mcp.CommandTransport{Command: exec.Command(c[0], c[1:]...)}
	}
	return nil
}

// command is the downstream server's command line: the one passed for this
// run, else the configured one.
func (r *Receiver) command(ctx context.Context) []string {
	if c, ok := ctx.Value(ctxKey{}).([]string); ok && len(c) > 0 {
		return c
	}
	return r.cfg.Command
}

// serverID names a downstream server in the stats and the lazy choices: the
// name it reports plus a short hash of its command line, so two
// configurations of one server binary stay apart. Only the hash is kept,
// never the command: arguments can hold keys.
func serverID(name string, command []string) string {
	if len(command) == 0 {
		return name
	}
	sum := sha256.Sum256([]byte(strings.Join(command, "\x00")))
	return name + "#" + hex.EncodeToString(sum[:4])
}

// forward calls the downstream tool and optimizes its text results.
func (r *Receiver) forward(cs *mcp.ClientSession, name string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return r.call(ctx, cs, name, req.Params.Arguments, req)
	}
}

// call runs a downstream tool and optimizes its text results.
func (r *Receiver) call(ctx context.Context, cs *mcp.ClientSession, name string, args any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	r.recordCall(ctx, req, name)
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	for i, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		p := payload.New(payload.ToolResult, tc.Text)
		p.Set(payload.AttrTool, name)
		r.sessions.Tag(p, req)
		out, err := r.next.Process(ctx, p)
		if err != nil {
			r.log.Warn("optimize tool result, passing original", "tool", name, "err", err)
			continue
		}
		res.Content[i] = &mcp.TextContent{Text: mcptool.Text(out, r.cfg.Retrieve), Annotations: tc.Annotations}
	}
	return res, nil
}
