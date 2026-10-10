// Package bunoptimizer compresses bun output: install progress dropped,
// bun test reduced to failures and the summary.
package bunoptimizer

import (
	"context"
	"regexp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct{ component.Nop }

var progress = regexp.MustCompile(`^\[\d+(\.\d+)?m?s\] |^Extracted |^Resolving dependencies|^Resolved, downloaded and extracted|^Saved lockfile`)

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		text := compress.StripANSI(b.Text)
		var lines []string
		for _, l := range compress.Lines(text) {
			if !progress.MatchString(l) {
				lines = append(lines, l)
			}
		}
		if compress.HasJSTest(text) {
			lines = compress.JSTest(lines)
		}
		return compress.Join(compress.SqueezeBlank(lines)), nil
	}, detect.BunTest, detect.BunInstall)
}
