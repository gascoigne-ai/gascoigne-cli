package mcptool

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestSessionsExplicitIdentity(t *testing.T) {
	id := Identity{Session: "s1", Agent: "claude-code", Model: "opus"}
	s := NewSessions(id)
	if s.Explicit() != id {
		t.Fatalf("Explicit = %+v, want %+v", s.Explicit(), id)
	}
	if got := s.Of(nil); got != "s1" {
		t.Errorf("Of = %q, want s1", got)
	}
	if got := s.Agent(nil); got != "claude-code" {
		t.Errorf("Agent = %q, want claude-code", got)
	}
	body := payload.New(payload.ToolResult, "text")
	s.Tag(body, nil)
	if body.Attrs[payload.AttrSession] != "s1" || body.Attrs[payload.AttrAgent] != "claude-code" || body.Attrs[payload.AttrModel] != "opus" {
		t.Fatalf("Tag set %v", body.Attrs)
	}
}

func TestSessionsGeneratedIdentity(t *testing.T) {
	s := NewSessions(Identity{})
	first := s.Of(nil)
	if !strings.HasPrefix(first, "mcp-") {
		t.Fatalf("Of = %q, want a generated mcp- id", first)
	}
	if s.Of(nil) != first {
		t.Errorf("the generated id changes between calls")
	}
	if got := s.Agent(nil); got != "" {
		t.Errorf("Agent = %q, want empty without a request", got)
	}
}
