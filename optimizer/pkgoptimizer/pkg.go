// Package pkgoptimizer is the package manager family engine (npm, pnpm,
// yarn, bun, pip, uv, poetry, go mod, cargo, apt/dnf/brew/pacman): progress
// and downloads go, warnings fold by kind, summaries and errors stay.
// Dependency trees (`tree`, `gomodgraph`) go through internal/compress/tree,
// inventories (`list`) through internal/compress/pkglist.
package pkgoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/pkglist"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/pkgnoise"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/tree"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	topts := tree.Options{MaxDepth: o.cfg.TreeDepth, Query: sample.Terms(data.Attrs[payload.AttrQuery])}
	return data.RewriteFamily(func(block payload.Block) (string, error) {
		dialect := detect.Dialect(block.Type)
		switch dialect {
		case "tree":
			return compress.Join(tree.Compress(compress.CleanLines(block.Text), topts)), nil
		case "list":
			return compress.Join(pkglist.Compress(compress.CleanLines(block.Text))), nil
		case "gomodgraph":
			return compress.Join(tree.Edges(compress.CleanLines(block.Text))), nil
		}
		r, ok := pkgnoise.For(dialect)
		if !ok {
			return block.Text, nil
		}
		return compress.Join(pkgnoise.Compress(compress.CleanLines(block.Text), r)), nil
	}, detect.FamilyPkg)
}
