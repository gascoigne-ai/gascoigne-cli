// Package gitoptimizer is the vcs family engine: git status, log, diff/show,
// push/pull/fetch, blame and branch output, and GitHub CLI lists and views.
package gitoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	query := sample.Terms(data.Attrs[payload.AttrQuery])
	return data.Rewrite(func(block payload.Block) (string, error) {
		lines := compress.CleanLines(block.Text)
		switch block.Type {
		case detect.GitStatus:
			lines = status(lines)
		case detect.GitLog:
			lines = o.log(lines)
		case detect.Diff:
			lines = o.diff(lines)
		case detect.GitRemote:
			lines = remote(lines)
		case detect.GitStat:
			lines = o.stat(lines)
		case detect.GitBlame:
			lines = blame(lines)
		case detect.GitBranch:
			lines = o.branch(lines)
		case detect.GitRefs:
			lines = refs(lines)
		case detect.GHList:
			lines = o.ghList(lines, query)
		case detect.GHView:
			lines = ghView(compress.StripANSI(block.Text))
		}
		return compress.Join(lines), nil
	}, detect.GitStatus, detect.GitLog, detect.Diff, detect.GitRemote, detect.GitStat, detect.GitBlame, detect.GitBranch, detect.GitRefs, detect.GHList, detect.GHView)
}

var progress = pcomb.Rules{
	{
		Name: "transfer progress",
		Match: pcomb.Any(
			pcomb.Word("Enumerating", "Counting", "Compressing", "Writing", "Receiving", "Resolving", "Delta compression", "Unpacking", "Updating files", "Checking out files"),
			pcomb.Prefixed("remote: ", pcomb.Word("Enumerating", "Counting", "Compressing", "Writing", "Receiving", "Resolving", "Delta compression", "Unpacking", "Updating files", "Checking out files")),
		),
		Yes: []string{"Enumerating objects: 5, done.", "remote: Counting objects: 100% (5/5), done.", "Writing objects: 100% (3/3), 300 bytes | 300.00 KiB/s, done.", "Delta compression using up to 8 threads"},
		No:  []string{"Countingx", "Writing_", "Receivingx objects", "From github.com:x/y"},
	},
	{
		Name:  "pack total",
		Match: pcomb.Re(`^(remote: )?Total \d+\b`),
		Yes:   []string{"Total 3 (delta 1), reused 0 (delta 0), pack-reused 0", "remote: Total 3 (delta 1)"},
		No:    []string{"Total", "Total x", "Total 35x"},
	},
	{
		Name:  "empty remote line",
		Match: pcomb.Prefixed("remote:", pcomb.Trimmed(pcomb.Exact(""))),
		Yes:   []string{"remote:", "remote: ", "remote:   "},
		No:    []string{"remote: Create a pull request", "remote:x"},
	},
}

// remote drops transfer progress of push, pull, fetch and clone.
func remote(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !progress.Match(l) {
			out = append(out, l)
		}
	}
	return out
}
