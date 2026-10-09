package catoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
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
	got := opt(Outline).rewrite(goSrc, detect.LangGo, Outline)
	want := `// outline: 1 bodies elided (cat_mode=full for complete source)
package x

// Add sums.
func Add(a, b int) int { … 3 lines }

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
	got := opt(Outline).rewrite(src, detect.LangCLike, Outline)
	for _, want := range []string{"export class Cart {", "  total(items) {", "    … 3 lines", "function helper() {", "  … 2 lines"} {
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
	got := opt(Outline).rewrite(src, detect.LangPython, Outline)
	for _, want := range []string{"class Decoder:", "    def decode(self, s,", `        """Decode s."""`, "        … 3 lines", "def main():", "    … 2 lines"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestAutoModes(t *testing.T) {
	o := opt(Auto)
	small := "package x\n"
	if got := o.rewrite(small, detect.LangGo, Auto); got != small {
		t.Errorf("small changed: %q", got)
	}
	if got := o.rewrite(goSrc, detect.LangGo, Auto); strings.Contains(got, "Copyright") || strings.Contains(got, "outline") {
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
