// Package gooptimizer compresses go test, go build and go vet output: passing
// tests and packages collapse to counts, failures keep their output, panics
// keep only frames outside the standard library.
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
		lines := compress.Lines(compress.StripANSI(b.Text))
		switch b.Type {
		case detect.GoTestJSON:
			text, err := jsonToText(b.Text)
			if err != nil {
				return "", err
			}
			lines = compress.Lines(text)
		case detect.GoBuild:
			return compress.Join(compress.DedupeAll(lines)), nil
		}
		return compress.Join(summarize(lines)), nil
	}, detect.GoTest, detect.GoTestJSON, detect.GoBuild)
}
