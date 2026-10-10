// Package gooptimizer compresses go build and go vet output (deduped).
// Test output moved to the test family (compress/testreport).
package gooptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		return compress.Join(compress.DedupeAll(compress.Lines(compress.StripANSI(b.Text)))), nil
	}, detect.GoBuild)
}
