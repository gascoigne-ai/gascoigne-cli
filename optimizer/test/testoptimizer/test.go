// Package testoptimizer is the test family engine: output of any test runner
// (go, jest, vitest, bun, mocha, deno, pytest, cargo, rspec, maven, dotnet)
// keeps failures with details and summaries; passes collapse to a count.
package testoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/testreport"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyTest {
			continue
		}
		lines, err := o.compressBlock(b)
		if err != nil {
			return err
		}
		p.Blocks[i].Text = compress.Join(lines)
	}
	return nil
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
		return testreport.Go(compress.Lines(compress.StripANSI(b.Text))), nil
	default:
		lines := compress.Lines(compress.StripANSI(b.Text))
		out, ok := testreport.Compress(dialect, lines)
		if !ok {
			return lines, nil
		}
		return out, nil
	}
}
