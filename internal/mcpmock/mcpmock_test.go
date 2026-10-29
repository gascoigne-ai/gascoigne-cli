package mcpmock

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

func TestCallIsDeterministic(t *testing.T) {
	for _, tool := range Tools {
		t.Run(tool.Name, func(t *testing.T) {
			a, err := Call(t.Context(), tool.Name)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := Call(t.Context(), tool.Name)
			if a == "" || a != b {
				t.Errorf("two calls differ or are empty (%d, %d bytes)", len(a), len(b))
			}
			if a != tool.Text() {
				t.Error("protocol result differs from the generator")
			}
		})
	}
}

func TestSizes(t *testing.T) {
	small, _ := Call(t.Context(), "get_status")
	big, _ := Call(t.Context(), "list_issues")
	if tokens.Estimate(small) > 50 || tokens.Estimate(big) < 2000 {
		t.Errorf("get_status %d tokens, list_issues %d: want a tiny and a large result", tokens.Estimate(small), tokens.Estimate(big))
	}
}
