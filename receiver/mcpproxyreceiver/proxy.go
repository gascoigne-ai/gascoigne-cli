// Package mcpproxyreceiver sits between an agent and another MCP server:
// it mirrors the server's tools and runs their text results through the
// pipeline, adding a retrieve tool for originals.
package mcpproxyreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress/schema"
	"github.com/lukaszb/gascoigne/mcptool"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/version"
)

type Receiver struct {
	cfg      *Config
	next     component.Pipeline
	log      *slog.Logger
	ledger   component.Ledger
	upstream mcp.Transport
	sessions *mcptool.Sessions
}

type ctxKey struct{}

// With sets the downstream server command for this run, overriding config.
func With(ctx context.Context, command []string) context.Context {
	return context.WithValue(ctx, ctxKey{}, command)
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
	s := mcp.NewServer(info, &mcp.ServerOptions{Instructions: init.Instructions, Logger: r.log})
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
	if r.cfg.MaxTools > 0 && len(tools) > r.cfg.MaxTools {
		r.addLazy(s, cs, tools)
	} else {
		for _, tool := range tools {
			s.AddTool(tool, r.forward(cs, tool.Name))
		}
	}
	mcptool.AddRetrieve(s, r.cfg.Retrieve, r.ledger)
	return s.Run(ctx, r.upstream)
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
	switch v := ctx.Value(ctxKey{}).(type) {
	case mcp.Transport:
		return v
	case []string:
		if len(v) > 0 {
			return &mcp.CommandTransport{Command: exec.Command(v[0], v[1:]...)}
		}
	}
	if c := r.cfg.Command; len(c) > 0 {
		return &mcp.CommandTransport{Command: exec.Command(c[0], c[1:]...)}
	}
	return nil
}

// forward calls the downstream tool and optimizes its text results.
func (r *Receiver) forward(cs *mcp.ClientSession, name string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return r.call(ctx, cs, name, req.Params.Arguments, req)
	}
}

// call runs a downstream tool and optimizes its text results.
func (r *Receiver) call(ctx context.Context, cs *mcp.ClientSession, name string, args any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
