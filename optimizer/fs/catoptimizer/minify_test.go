package catoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
)

func clean(src, lang string, h detect.Hints) string {
	return opt(Clean).rewrite(src, lang, h, Clean)
}

func TestCleanKeepsLinesVerbatim(t *testing.T) {
	src := `package x

// Total adds prices of all items in the cart and applies the discount
// configured for the customer; negative totals are clamped to zero.
// See pricing.md for the rules behind discounts.
func Total(items []int) int {
	t := 0 // running sum
	return t
}
`
	got := clean(src, detect.LangGo, detect.Hints{})
	for _, l := range compress.Lines(got) {
		if !strings.Contains(src, l) && !strings.Contains(l, "// … ") {
			t.Errorf("line changed: %q", l)
		}
	}
	for _, want := range []string{"// Total adds prices", "// … comments L4-5", "	t := 0 // running sum"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestCleanLanguages(t *testing.T) {
	tests := []struct {
		name, src, lang string
		h               detect.Hints
		keep, drop      []string
	}{
		{
			name: "python: # inside a multi-line string is not a comment",
			src: `SQL = """
# not a comment, part of the query text and long enough to be dropped
# still the query text, a second line that must stay as it is
"""
x = 1
`,
			lang: detect.LangPython,
			keep: []string{"# not a comment", "# still the query"},
		},
		{
			name: "python: docstring summary stays, body never empty",
			src: `def f():
    """Return one.

    A long explanation of why this returns one, spanning several lines
    of prose that an agent rarely needs when reading the code.
    """
x = 1
`,
			lang: detect.LangPython,
			keep: []string{`    """Return one.`, "    # … docstring L3-6"},
			drop: []string{"long explanation"},
		},
		{
			name: "ruby: rdoc a blank line above its method keeps its summary",
			src: `require "json"

class Cart
  # Total price of every item, in cents, after discounts are applied
  # and before tax. Discounts come from the active promotion, if any,
  # and never take the total below zero.

  def total
    items.sum(&:price)
  end
end
`,
			lang: detect.LangOther,
			h:    detect.Hints{File: "cart.rb"},
			keep: []string{"  # Total price of every item", "  # … comments L5-6"},
			drop: []string{"never take the total"},
		},
		{
			name: "python: string argument is not a docstring",
			src: `run(
    """
    # a template line inside a string literal that must survive here
    # and a second line of the same template text, also kept as is
    """)
`,
			lang: detect.LangPython,
			keep: []string{"# a template line", "# and a second"},
		},
		{
			name: "directives stay",
			src: `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
# type: ignore
import os
`,
			lang: detect.LangPython,
			keep: []string{"#!/usr/bin/env", "coding: utf-8", "# type: ignore"},
		},
		{
			name: "go: build tags stay",
			src: `//go:build linux && amd64

package x
`,
			lang: detect.LangGo,
			keep: []string{"//go:build linux"},
		},
		{
			name: "shell: heredoc lines are text",
			src: `cat <<EOF
# a heading inside the heredoc that is long enough to be dropped
# another heading line inside the heredoc, kept as text too
EOF
`,
			lang: detect.LangShell,
			keep: []string{"# a heading", "# another heading"},
		},
		{
			name: "javadoc: first line with words stays",
			src: `/**
 * Returns the total of all items, after discounts were applied
 * by the pricing service for this customer and region.
 */
public int total() {
`,
			lang: detect.LangCLike,
			keep: []string{" * Returns the total", "/**"},
			drop: []string{"pricing service"},
		},
		{
			name: "block comment followed by code stays",
			src: `/* a block comment that is long enough to drop */ int x = 1;
/* and another one closing with code after it */ int y = 2;
`,
			lang: detect.LangCLike,
			keep: []string{"int x = 1;", "int y = 2;"},
		},
		{
			name: "ruby by extension",
			src: `# A long header comment for this ruby file that explains things
# in more words than any agent needs while it reads the code.

puts "hi"
`,
			lang: detect.LangOther,
			h:    detect.Hints{File: "app.rb"},
			keep: []string{"# … comments L1-2", `puts "hi"`},
		},
		{
			name: "tail output has no file line numbers",
			src: `// A long comment block at the end of a file, long enough to drop it,
// with a second line that also goes away in the minified output.

func f() {}
`,
			lang: detect.LangCLike,
			h:    detect.Hints{Cmd: "tail -n 50 x.js"},
			keep: []string{"// … comments 2 lines"},
		},
		{
			name: "short comments cost less than a marker",
			src: `x := 1
// short

y := 2
`,
			lang: detect.LangGo,
			keep: []string{"// short"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clean(tt.src, tt.lang, tt.h)
			for _, want := range tt.keep {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
			for _, gone := range tt.drop {
				if strings.Contains(got, gone) {
					t.Errorf("kept %q in:\n%s", gone, got)
				}
			}
		})
	}
}

func TestUnknownLanguageUntouched(t *testing.T) {
	src := "; some lisp-ish comment that is long enough to be dropped, really\n(defun f () 1)\n"
	if got := clean(src, "", detect.Hints{}); got != src {
		t.Fatalf("changed:\n%s", got)
	}
}
