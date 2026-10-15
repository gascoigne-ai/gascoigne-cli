// Package buildoptimizer is the build family engine: compiler, linter, type
// checker and bundler output (go, tsc, eslint, golangci-lint, ruff, mypy,
// rustc, gcc/clang, javac, maven, dotnet, vite/webpack/rollup/esbuild, docker
// build) becomes diagnostics grouped by file with tool noise dropped.
package buildoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/compose"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/diag"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/watch"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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
		dialect := detect.Dialect(b.Type)
		r := diag.For(dialect)
		if r.MaxFrame == 0 {
			r.MaxFrame = o.cfg.MaxFrame
		}
		var lines []string
		switch dialect {
		case "sarif":
			var err error
			if lines, err = diag.SARIF(b.Text); err != nil {
				return err
			}
		case "docker":
			lines = dockerSteps(watch.Last(compress.CleanLines(b.Text)))
		default:
			lines = watch.Last(compress.CleanLines(b.Text))
		}
		p.Blocks[i].Text = compress.Join(diag.Compress(lines, r))
	}
	return nil
}

var (
	dockerStep = lazyre.New(`^#(\d+) \[[^\]]*\d+/\d+\] RUN (.+)$`)
	dockerOut  = lazyre.New(`^#(\d+) \d+\.\d+ ?(.*)$`)
)

// dockerSteps compresses each RUN step's output as the tool it runs (npm ci
// as pkg, tsc as build…), keeping step headers and everything else.
func dockerSteps(lines []string) []string {
	cmds := map[string]string{}
	bodies := map[string][]string{}
	steps := make([][]string, len(lines))
	outs := make([][]string, len(lines))
	for i, l := range lines {
		if steps[i] = dockerStep.FindStringSubmatch(l); steps[i] != nil {
			cmds[steps[i][1]] = steps[i][2]
		} else if outs[i] = dockerOut.FindStringSubmatch(l); outs[i] != nil && cmds[outs[i][1]] != "" {
			bodies[outs[i][1]] = append(bodies[outs[i][1]], outs[i][2])
		}
	}
	var out []string
	for i, l := range lines {
		if m := outs[i]; m != nil && cmds[m[1]] != "" {
			continue // replaced by the step's compressed body
		}
		out = append(out, l)
		if m := steps[i]; m != nil {
			for _, b := range compose.Lines(bodies[m[1]], m[2]) {
				out = append(out, "#"+m[1]+" "+b)
			}
		}
	}
	return out
}
