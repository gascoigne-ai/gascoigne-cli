// Package pkgoptimizer is the package manager family engine (npm, pnpm,
// yarn, bun, pip, uv, poetry, go mod, cargo, apt/dnf/brew/pacman): progress
// and downloads go, warnings fold by kind, summaries and errors stay.
package pkgoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/compress/pkgnoise"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyPkg {
			continue
		}
		r, ok := pkgnoise.For(detect.Dialect(b.Type))
		if !ok {
			continue
		}
		p.Blocks[i].Text = compress.Join(pkgnoise.Compress(compress.Lines(compress.StripANSI(b.Text)), r))
	}
	return nil
}
