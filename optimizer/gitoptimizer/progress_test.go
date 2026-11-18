package gitoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

func TestProgressRulesVerify(t *testing.T) {
	if err := progress.Verify(); err != nil {
		t.Fatal(err)
	}
}

// The regexp progress replaced, kept to pin the old behavior.
var oldProgress = lazyre.New(`^(remote: )?(Enumerating|Counting|Compressing|Writing|Receiving|Resolving|Delta compression|Total \d+|Unpacking|Updating files|Checking out files)\b|^remote:\s*$`)

func TestProgressSameAsRegexp(t *testing.T) {
	var ex []string
	for _, r := range progress {
		ex = append(ex, r.Yes...)
		ex = append(ex, r.No...)
	}
	ex = append(ex, "Enumerating objects: 5, done.", "remote: Enumerating objects: 5, done.", "remote: Counting objects: 100% (5/5), done.", "Counting objects", "Counting", "Countingx", "Counting_", "Counting-x",
		"Compressing objects: 100% (3/3), done.", "Writing objects: 100% (3/3), 300 bytes | 300.00 KiB/s, done.", "Receiving objects: 100% (10/10), done.", "Resolving deltas: 100% (2/2), done.",
		"Delta compression using up to 8 threads", "Delta compression", "Delta compressions", "Total 3 (delta 1), reused 0 (delta 0)", "remote: Total 3 (delta 1)", "Total 3", "Total 35x", "Total 3_", "Total x", "Total ",
		"Unpacking objects: 100% (3/3), done.", "Updating files: 100% (10/10), done.", "Checking out files: 100%", "Updating filesx", "remote:", "remote: ", "remote:   ", "remote:\t", "remote: x", "remote:x",
		"remote: remote: Counting", "remote:  Counting", "remoteCounting", "From github.com:x/y", " * [new branch]      main -> origin/main", "Already up to date.")
	sameAsOld(t, oldProgress.MatchString, progress.Match, ex)
}
