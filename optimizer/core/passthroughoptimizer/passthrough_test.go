package passthroughoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestOptimizeKeepsContent(t *testing.T) {
	p := payload.New(payload.CmdOutput, "x")
	orig := p.Clone()
	if err := (Optimizer{}).Optimize(t.Context(), p); err != nil || !p.SameContent(orig) {
		t.Fatalf("changed: %+v, %v", p, err)
	}
}
