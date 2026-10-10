package docoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
)

func testOptimizer() *Optimizer {
	return &Optimizer{cfg: &Config{MaxCodeLines: 6, ManDrop: []string{"SEE ALSO"}}}
}

func TestMarkdown(t *testing.T) {
	in := `# Tool
[![CI](https://github.com/a/b/workflows/ci/badge.svg)](https://github.com/a/b/actions)
<p align="center"><img src="logo.png"></p>
<!-- hidden
note -->
- [Install](#install)
- [Usage](#usage)

![diagram](docs/arch.png)

## Install
` + "```sh\n1\n2\n3\n4\n5\n6\n7\n8\n9\n```\n"
	got := compress.Join(testOptimizer().markdown(compress.Lines(htmlComment.ReplaceAllString(in, ""))))
	want := "# Tool\n\n[image: diagram]\n\n## Install\n```sh\n1\n2\n3\n4\n… 3 lines omitted\n8\n9\n```\n… 2 table of contents entries omitted\n"
	if got := compress.Join(compress.SqueezeBlank(compress.Lines(got))); got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestMan(t *testing.T) {
	in := "LS(1)\t\t\t User Commands\t\t\t  LS(1)\n\nNAME\n       ls - list  directory contents\n\nSEE ALSO\n       dircolors(1)\n\nGNU coreutils 9.11\t\t   July 2026\t\t\t\t  LS(1)\n"
	got := compress.Join(testOptimizer().man(compress.Lines(in)))
	if got != "\nNAME\n    ls - list directory contents\n\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRST(t *testing.T) {
	in := ".. image:: https://ci/badge.svg\n   :target: https://ci\n\nTitle\n=====\n\n.. a comment\n   more\n\nText.\n"
	got := compress.Join(compress.SqueezeBlank(testOptimizer().rst(compress.Lines(in))))
	if strings.Contains(got, "badge") || strings.Contains(got, "comment") || !strings.Contains(got, "Title\n=====\n") || !strings.Contains(got, "Text.") {
		t.Fatalf("got:\n%s", got)
	}
}
