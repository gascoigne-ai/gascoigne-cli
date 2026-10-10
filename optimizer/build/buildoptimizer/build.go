// Package buildoptimizer is the build family engine: compiler, linter, type
// checker and bundler output (go, tsc, eslint, golangci-lint, ruff, mypy,
// rustc, gcc/clang, javac, maven, dotnet, vite/webpack/rollup/esbuild, docker
// build) becomes diagnostics grouped by file with tool noise dropped.
package buildoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/diag"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyBuild {
			continue
		}
		r := diag.For(detect.Dialect(b.Type))
		if r.MaxFrame == 0 {
			r.MaxFrame = o.cfg.MaxFrame
		}
		p.Blocks[i].Text = compress.Join(diag.Compress(compress.Lines(compress.StripANSI(b.Text)), r))
	}
	return nil
}
