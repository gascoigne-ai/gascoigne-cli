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

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	return data.RewriteFamily(func(block payload.Block) (string, error) {
		lines, err := o.compressBlock(block)
		if err != nil {
			return "", err
		}
		return compress.Join(lines), nil
	}, detect.FamilyTest)
}

func (o *Optimizer) compressBlock(block payload.Block) ([]string, error) {
	switch dialect := detect.Dialect(block.Type); dialect {
	case "junit":
		return testreport.JUnit(block.Text)
	case "coverage":
		return testreport.Coverage(compress.Lines(block.Text), o.cfg.CoverageBelow), nil
	case "go-json":
		return testreport.GoJSON(block.Text)
	case "go":
		return testreport.Go(compress.CleanLines(block.Text)), nil
	default:
		lines := testreport.CoverageTables(compress.CleanLines(block.Text), o.cfg.CoverageBelow)
		out, ok := testreport.Compress(dialect, lines)
		if !ok {
			return lines, nil
		}
		return out, nil
	}
}
