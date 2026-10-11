package docoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
)

func testOptimizer() *Optimizer {
	return &Optimizer{cfg: &Config{MaxCodeLines: 6, MaxTokens: 4000, ManDrop: []string{"SEE ALSO"}}}
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
	got := compress.Join(testOptimizer().markdown(compress.Lines(htmlComment.ReplaceAllString(in, "")), ""))
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

func TestRSTBadgesAndTargets(t *testing.T) {
	in := "| |pypi| |ci|\n\nIntro with |version| and `docs`_.\n\n.. |pypi| image:: https://img.shields.io/pypi/v/x.svg\n    :target: https://pypi.org/x\n" +
		".. |ci| image:: https://ci/badge.svg\n.. |version| replace:: 1.2\n.. _docs: https://x.readthedocs.io\n"
	got := compress.Join(compress.SqueezeBlank(testOptimizer().rst(compress.Lines(in))))
	for _, want := range []string{"Intro with |version| and `docs`_.", ".. |version| replace:: 1.2", ".. _docs: https://x.readthedocs.io"} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "|pypi|") || strings.Contains(got, "shields") {
		t.Errorf("badges kept:\n%s", got)
	}
}

func TestSections(t *testing.T) {
	filler := strings.Repeat("lorem ipsum dolor sit amet consectetur\n", 20)
	in := "# Page\nintro\n## Alpha\n" + filler + "### Alpha detail\n" + filler +
		"## Beta\nthe widget setting lives here\n" + filler + "## Gamma\n```\n## not a heading\n```\n" + filler
	lines := compress.Lines(in)
	tests := []struct {
		name, query string
		budget      int
		keep, drop  []string
	}{
		{"under budget", "", 10000, []string{"## not a heading", "widget"}, []string{"omitted"}},
		{"page order", "", 200, []string{"intro", "## Alpha\nlorem", "## Beta\n… 21 lines omitted", "## Gamma\n… 23 lines omitted"},
			[]string{"widget", "### Alpha detail"}},
		{"query first", "widget settings", 200, []string{"intro", "## Alpha\n… 41 lines omitted", "## Beta\nthe widget setting"},
			[]string{"### Alpha detail"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compress.Join(sections(lines, tt.query, tt.budget))
			for _, k := range tt.keep {
				if !strings.Contains(got, k) {
					t.Errorf("missing %q in:\n%s", k, got)
				}
			}
			for _, d := range tt.drop {
				if strings.Contains(got, d) {
					t.Errorf("unexpected %q in:\n%s", d, got)
				}
			}
		})
	}
}

func TestWeakMatchesWaitTheirTurn(t *testing.T) {
	filler := strings.Repeat("lorem ipsum dolor sit amet consectetur\n", 10)
	in := "# Page\n## Alpha\n" + filler + "## Beta\nwidget v1.2 setting\n" + filler + "## Gamma\none widget\n" + filler
	got := compress.Join(sections(compress.Lines(in), "widget setting v1.2", 180))
	if !strings.Contains(got, "## Alpha\nlorem") || !strings.Contains(got, "## Beta\nwidget") || strings.Contains(got, "one widget") {
		t.Fatalf("want Beta by relevance, then Alpha by page order, not the weak Gamma:\n%s", got)
	}
}

func TestHelpQuery(t *testing.T) {
	var lines []string
	lines = append(lines, "Usage: tool [options]")
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("  --flag%d    does thing %d", i, i))
	}
	lines = append(lines, "  --proxy <url>   use this proxy", "      for every request", "  --timeout <s>   give up after s")
	got := strings.Join(helpFor(lines, "proxy"), "\n")
	for _, want := range []string{"Usage: tool [options]", "--proxy <url>", "for every request", "… 30 options omitted", "… 1 option omitted"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if same := helpFor(lines, ""); len(same) != len(lines) {
		t.Fatal("no query changed the help")
	}
}

func TestHugeBestSectionKeepsItsHead(t *testing.T) {
	filler := strings.Repeat("lorem ipsum dolor sit amet consectetur\n", 200)
	in := "# Page\n## Events\nPostToolUse can replace output.\n" + filler + "## Small\nshort text\n"
	got := compress.Join(sections(compress.Lines(in), "PostToolUse replace", 200))
	for _, want := range []string{"## Events\nPostToolUse can replace output.", "lines omitted", "## Small"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// An article under one heading is one section over the budget: without a
// query it used to keep only its title.
func TestOneHugeSectionKeepsItsHead(t *testing.T) {
	filler := strings.Repeat("lorem ipsum dolor sit amet consectetur\n", 200)
	in := "# Article title\nFirst paragraph of the story.\n" + filler + "Last paragraph.\n"
	got := compress.Join(sections(compress.Lines(in), "", 200))
	if !strings.Contains(got, "# Article title\nFirst paragraph of the story.\nlorem") || !strings.Contains(got, "lines omitted") {
		t.Fatalf("want the head of the article:\n%s", got)
	}
	if strings.Contains(got, "Last paragraph.") {
		t.Fatalf("over budget, the tail should go:\n%s", got)
	}
}
