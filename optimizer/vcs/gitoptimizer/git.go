// Package gitoptimizer is the vcs family engine: git status, log, diff/show,
// push/pull/fetch, blame and branch output, and GitHub CLI lists and views.
package gitoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := rows.Terms(p.Attrs[payload.AttrQuery])
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
		case detect.GitBlame:
			lines = blame(lines)
		case detect.GitBranch:
			lines = o.branch(lines)
		case detect.GHList:
			lines = o.ghList(lines, query)
		case detect.GHView:
			lines = ghView(compress.StripANSI(b.Text))
		}
		return compress.Join(lines), nil
	}, detect.GitStatus, detect.GitLog, detect.Diff, detect.GitRemote, detect.GitBlame, detect.GitBranch, detect.GHList, detect.GHView)
}

var progress = lazyre.New(`^(remote: )?(Enumerating|Counting|Compressing|Writing|Receiving|Resolving|Delta compression|Total \d+|Unpacking|Updating files|Checking out files)\b|^remote:\s*$`)

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
