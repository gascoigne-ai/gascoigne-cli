package mcptool

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type identityKey struct{}

// Identity is who tool calls run for; empty fields are resolved per call.
type Identity struct{ Session, Agent, Model string }

// WithIdentity sets explicit identity (e.g. from CLI flags) for MCP servers run with ctx.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

// Sessions resolves the identity of tool calls: explicit values from the
// context first; else the MCP transport's session id (or one generated per
// server run) and the client's name as agent.
type Sessions struct {
	explicit  Identity
	generated string
}

func NewSessions(ctx context.Context) *Sessions {
	explicit, _ := ctx.Value(identityKey{}).(Identity)
	return &Sessions{explicit: explicit, generated: fmt.Sprintf("mcp-%s-%04x", time.Now().Format("20060102-150405"), rand.N(1<<16))}
}

func (s *Sessions) Of(req *mcp.CallToolRequest) string {
	if s.explicit.Session != "" {
		return s.explicit.Session
	}
	if req != nil && req.Session != nil && req.Session.ID() != "" {
		return req.Session.ID()
	}
	return s.generated
}

// Explicit is the identity given with -session, -agent and -model.
func (s *Sessions) Explicit() Identity { return s.explicit }

// Agent is the explicit agent, else the MCP client's name.
func (s *Sessions) Agent(req *mcp.CallToolRequest) string {
	if s.explicit.Agent != "" || req == nil {
		return s.explicit.Agent
	}
	if info := req.ClientInfo(); info != nil {
		return info.Name
	}
	return ""
}

// Tag sets session, agent and model attrs on p for a tool call.
func (s *Sessions) Tag(body *payload.Payload, req *mcp.CallToolRequest) {
	body.Set(payload.AttrSession, s.Of(req))
	body.Set(payload.AttrAgent, s.Agent(req))
	body.Set(payload.AttrModel, s.explicit.Model)
}
