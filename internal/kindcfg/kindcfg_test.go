package kindcfg

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestValidate(t *testing.T) {
	if err := (&Config{}).Validate(); err == nil || err.Error() != "kind must be set" {
		t.Errorf("empty kind: %v", err)
	}
	if err := (&Config{Kind: payload.ToolResult}).Validate(); err != nil {
		t.Errorf("set kind: %v", err)
	}
}
