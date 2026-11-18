package mcpreceiver

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

func TestFactoryAppliesIdentity(t *testing.T) {
	id := mcptool.Identity{Session: "s1", Agent: "claude-code", Model: "opus"}
	f := NewFactory(WithIdentity(id))
	c, err := f.Create(t.Context(), component.Settings{}, f.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if got := c.(*Receiver).identity; got != id {
		t.Fatalf("identity %+v, want %+v", got, id)
	}
}
