package mcptool

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Identity is who tool calls run for; empty fields are resolved per call.
type Identity struct{ Session, Agent, Model string }

// Sessions resolves the identity of tool calls: explicit values given to
// NewSessions first; else the MCP transport's session id (or one generated per
// server run) and the client's name as agent.
type Sessions struct {
	explicit  Identity
	generated string
}

// NewSessions returns a Sessions that falls back to one id generated per server run
// when the explicit identity names no session.
func NewSessions(explicit Identity) *Sessions {
	return &Sessions{explicit: explicit, generated: fmt.Sprintf("mcp-%s-%04x", time.Now().Format("20060102-150405"), rand.N(1<<16))}
}

// Of returns the session id for a tool request.
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
