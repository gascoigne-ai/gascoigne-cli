package catoptimizer

import (
	"slices"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func opt(mode string) *Optimizer {
	return &Optimizer{cfg: &Config{Mode: mode, CleanMinTokens: 10, OutlineMinTokens: 1000, MinBody: 2}}
}

const goSrc = `// Copyright 2026 The Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license.

package x

// Add sums.
func Add(a, b int) int {
	s := a
	s += b
	return s
}

type T struct{ n int }

func (t *T) Get() int { return t.n }
`

func TestGoOutline(t *testing.T) {
	got := opt(Outline).rewrite(goSrc, detect.LangGo, detect.Hints{}, Outline)
	want := `// outline: 1 bodies elided (cat_mode=full for complete source)
// … comments L1-2

package x

// Add sums.
func Add(a, b int) int {
	// … L8-10
}

type T struct{ n int }

func (t *T) Get() int { return t.n }
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBraceOutline(t *testing.T) {
	src := `import x from "y";
export class Cart {
  total(items) {
    let s = "{";
    for (const i of items) { s += i; }
    return s;
  }
}
function helper() {
  a();
  b();
}
`
	got := opt(Outline).rewrite(src, detect.LangCLike, detect.Hints{}, Outline)
	for _, want := range []string{"export class Cart {", "  total(items) {", "    // … L4-6", "function helper() {", "  // … L10-11"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "return s") {
		t.Errorf("body kept:\n%s", got)
	}
}

func TestPythonOutline(t *testing.T) {
	src := `class Decoder:
    """Decodes."""

    def decode(self, s,
               strict=True):
        """Decode s."""
        obj = self.raw(s)
        check(obj)
        return obj

def main():
    run()
    exit()
`
	got := opt(Outline).rewrite(src, detect.LangPython, detect.Hints{}, Outline)
	for _, want := range []string{"class Decoder:", "    def decode(self, s,", `        """Decode s."""`, "        # … L7-9", "def main():", "    # … L12-13"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestAutoModes(t *testing.T) {
	o := opt(Auto)
	small := "package x\n"
	if got := o.rewrite(small, detect.LangGo, detect.Hints{}, Auto); got != small {
		t.Errorf("small changed: %q", got)
	}
	if got := o.rewrite(goSrc, detect.LangGo, detect.Hints{}, Auto); strings.Contains(got, "Copyright") || strings.Contains(got, "outline") {
		t.Errorf("medium should be cleaned only:\n%s", got)
	}
}

func TestModeOverride(t *testing.T) {
	p := &payload.Payload{Blocks: []payload.Block{{Text: goSrc, Type: detect.Code}}, Attrs: map[string]string{"file": "x.go", AttrMode: Full}}
	if err := opt(Outline).Optimize(t.Context(), p); err != nil || p.Blocks[0].Text != goSrc {
		t.Fatalf("full override ignored: %v\n%s", err, p.Blocks[0].Text)
	}
	t.Setenv(EnvMode, "bogus")
	p.Attrs[AttrMode] = ""
	if err := opt(Outline).Optimize(t.Context(), p); err == nil {
		t.Fatal("want error for invalid mode")
	}
}

// Only a keyword before a brace makes its block a container.
func TestBraceOutlineKeywordAfterBrace(t *testing.T) {
	lines := strings.Split(`const cfg = { a: 1 }; class Shop {
  total(items) {
    let s = 0
    for (const i of items) s += i
    return s
  }
}
run({ mode: "x" }, function main() {
  one()
  two()
  three()
})`, "\n")
	got := opt(Outline).braceOutline(lines)
	want := []span{{from: 2, to: 4}, {from: 8, to: 10}}
	if !slices.Equal(got, want) {
		t.Errorf("spans %v, want %v", got, want)
	}
}
