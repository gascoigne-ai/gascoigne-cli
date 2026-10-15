// Package testoptimizer is the test family engine: output of any test runner
// (go, jest, vitest, bun, mocha, deno, pytest, cargo, rspec, maven, dotnet)
// keeps failures with details and summaries; passes collapse to a count.
package testoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/testreport"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.RewriteFamily(func(b payload.Block) (string, error) {
		lines, err := o.compressBlock(b)
		if err != nil {
			return "", err
		}
		return compress.Join(lines), nil
	}, detect.FamilyTest)
}

func (o *Optimizer) compressBlock(b payload.Block) ([]string, error) {
	switch dialect := detect.Dialect(b.Type); dialect {
	case "junit":
		return testreport.JUnit(b.Text)
	case "coverage":
		return testreport.Coverage(compress.Lines(b.Text), o.cfg.CoverageBelow), nil
	case "go-json":
		return testreport.GoJSON(b.Text)
	case "go":
		return testreport.Go(compress.CleanLines(b.Text)), nil
	default:
		lines := compress.CleanLines(b.Text)
		out, ok := testreport.Compress(dialect, lines)
		if !ok {
			return lines, nil
		}
		return out, nil
	}
}
