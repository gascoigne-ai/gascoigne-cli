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
		lines := watch.Last(compress.Lines(compress.StripANSI(b.Text)))
		switch dialect {
		case "docker":
			lines = dockerSteps(lines)
		case "sarif":
			var err error
			if lines, err = diag.SARIF(b.Text); err != nil {
				return err
			}
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
	for _, l := range lines {
		if m := dockerStep.FindStringSubmatch(l); m != nil {
			cmds[m[1]] = m[2]
		} else if m := dockerOut.FindStringSubmatch(l); m != nil && cmds[m[1]] != "" {
			bodies[m[1]] = append(bodies[m[1]], m[2])
		}
	}
	var out []string
	for _, l := range lines {
		if m := dockerOut.FindStringSubmatch(l); m != nil && cmds[m[1]] != "" {
			continue // replaced by the step's compressed body
		}
		out = append(out, l)
		if m := dockerStep.FindStringSubmatch(l); m != nil {
			for _, b := range compose.Lines(bodies[m[1]], m[2]) {
				out = append(out, "#"+m[1]+" "+b)
			}
		}
	}
	return out
}
