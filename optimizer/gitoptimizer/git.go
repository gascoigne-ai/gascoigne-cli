// Package gitoptimizer compresses git status, log, diff/show and
// push/pull/fetch output.
package gitoptimizer

import (
	"context"
	"regexp"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		lines := compress.Lines(compress.StripANSI(b.Text))
		switch b.Type {
		case detect.GitStatus:
			lines = status(lines)
		case detect.GitLog:
			lines = o.log(lines)
		case detect.Diff:
			lines = o.diff(lines)
		case detect.GitRemote:
			lines = remote(lines)
		}
		return compress.Join(lines), nil
	}, detect.GitStatus, detect.GitLog, detect.Diff, detect.GitRemote)
}

var progress = regexp.MustCompile(`^(remote: )?(Enumerating|Counting|Compressing|Writing|Receiving|Resolving|Delta compression|Total \d+|Unpacking|Updating files|Checking out files)\b|^remote:\s*$`)

// remote drops transfer progress of push, pull, fetch and clone.
func remote(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !progress.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}
