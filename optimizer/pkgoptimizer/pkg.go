// Package pkgoptimizer is the package manager family engine (npm, pnpm,
// yarn, bun, pip, uv, poetry, go mod, cargo, apt/dnf/brew/pacman): progress
// and downloads go, warnings fold by kind, summaries and errors stay.
package pkgoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/pkgnoise"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.RewriteFamily(func(b payload.Block) (string, error) {
		r, ok := pkgnoise.For(detect.Dialect(b.Type))
		if !ok {
			return b.Text, nil
		}
		return compress.Join(pkgnoise.Compress(compress.CleanLines(b.Text), r)), nil
	}, detect.FamilyPkg)
}
