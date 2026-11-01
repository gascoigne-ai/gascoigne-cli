package tree

import (
	"os"
	"strings"
	"testing"
)

func lines(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

func fixture(t *testing.T, name string) []string {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/eval/pkg/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return lines(string(b))
}

func size(lines []string) int { return len(strings.Join(lines, "\n")) }

func TestCompress(t *testing.T) {
	tests := []struct {
		name string
		in   string
		o    Options
		want string
	}{
		{"leaves share a line, repeats go", `proj@1.0.0
├─┬ a@1
│ ├── b@1
│ ├── c@1 deduped
│ └── d@1
└── c@1
`, Options{}, `[repeats (deduped) omitted. name alone: the version listed first]
proj@1.0.0: c@1
  a@1: b@1, d@1`},
		{"a repeat that matches the query stays", `proj@1.0.0
├─┬ a@1
│ ├── b@1
│ ├── c@1 deduped
│ └── d@1
└── c@1
`, Options{Query: []string{"c@"}}, `[repeats (deduped) omitted. name alone: the version listed first]
proj@1.0.0: c@1
  a@1: b@1, d@1, =c`},
		{"a package with only repeats below is a leaf", `root
├─┬ a 1
│ └── x 1 (*)
├── b 1
└── x 1
`, Options{}, `[repeats (deduped) omitted. name alone: the version listed first]
root: a 1, b 1, x 1`},
		{"a package is expanded once", `root
├─┬ a 1
│ ├─┬ c 1
│ │ ├── x 1
│ │ └── y 1
│ └── z 1
└─┬ b 1
  ├─┬ c 1
  │ ├── x 1
  │ └── y 1
  └── q 1
`, Options{}, `[repeats (deduped) omitted. name alone: the version listed first]
root
  a 1: z 1
    c 1: x 1, y 1
  b 1: c, q 1`},
		{"a version is written once", `root
├── a@1.0.0
└─┬ b@2.0.0
  ├── a@1.0.0
  ├── c@1.0.0
  └── c@2.0.0
`, Options{}, `[repeats (deduped) omitted. name alone: the version listed first]
root: a@1.0.0
  b@2.0.0: a, c@1.0.0, c@2.0.0`},
		{"depth cap counts what it hides", `root
└─┬ a
  └─┬ b
    └─┬ c
      └── d
`, Options{MaxDepth: 2}, `root
  a
    b (+2 deeper)`},
		{"a query match is not cut", `root
├─┬ a
│ └─┬ b
│   └─┬ c
│     └─┬ d
│       └── wanted 1
└─┬ e
  └─┬ f
    └─┬ g
      └── h
`, Options{MaxDepth: 1, Query: []string{"wanted"}}, `root
  a
    b
      c
        d: wanted 1
  e (+3 deeper)`},
		{"not a tree stays", "plain\nlines\nonly\n", Options{}, "plain\nlines\nonly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(Compress(lines(tt.in), tt.o), "\n"); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestLongListWraps(t *testing.T) {
	in := "root\n"
	for i := range 12 {
		in += "├── a-rather-long-package-name-" + string(rune('a'+i)) + "@1.2.3\n"
	}
	for _, l := range Compress(lines(in), Options{}) {
		if len(l) > wrapAt+40 {
			t.Errorf("line of %d bytes: %q", len(l), l)
		}
	}
}

func TestPipdeptreeLabels(t *testing.T) {
	in := lines(`celery==5.6.3
├── kombu [required: >=5.6.0, installed: 5.6.2]
│   ├── amqp [required: >=5.1.1,<6.0.0, installed: 5.4.1]
│   └── vine [required: ==5.1.0, installed: 5.1.0]
└── tzlocal [required: Any, installed: 5.4.5, extra: tz]`)
	want := "celery==5.6.3: tzlocal 5.4.5 (extra: tz)\n  kombu 5.6.2: amqp 5.4.1, vine 5.1.0"
	if got := strings.Join(Compress(in, Options{}), "\n"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestEdges(t *testing.T) {
	in := []string{"app a@v1", "app b@v2", "a@v1 c@v3", "a@v1 b@v1"}
	want := "app: a@v1, b@v2\na@v1: c@v3, b@v1"
	if got := strings.Join(Edges(in), "\n"); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := Edges([]string{"app a@v1", "not an edge"}); len(got) != 2 || got[1] != "not an edge" {
		t.Errorf("non-edge text changed: %q", got)
	}
}

// Real tool output: each gets much smaller, and a second run changes nothing
// (the output has no glyphs left).
func TestFixtures(t *testing.T) {
	tests := []struct {
		file    string
		minSave float64 // of bytes
	}{
		{"npm-ls-all.txt", 0.6},
		{"pnpm-list-depth.txt", 0.55},
		{"pipdeptree.txt", 0.6},
		{"uv-tree.txt", 0.4},
		{"poetry-show-tree.txt", 0.3},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			in := fixture(t, tt.file)
			out := Compress(in, Options{MaxDepth: 5})
			if saved := 1 - float64(size(out))/float64(size(in)); saved < tt.minSave {
				t.Errorf("saved %.0f%%, want %.0f%%", saved*100, tt.minSave*100)
			}
			if again := Compress(out, Options{MaxDepth: 5}); strings.Join(again, "\n") != strings.Join(out, "\n") {
				t.Error("second run changed the output")
			}
		})
	}
}

// The depth cap hides jackspeak (under test-exclude and glob, below the cap
// of 5) until the query names it; then its path stays.
func TestQueryReachesPastTheDepthCap(t *testing.T) {
	in := fixture(t, "npm-ls-all.txt")
	if out := strings.Join(Compress(in, Options{MaxDepth: 5}), "\n"); strings.Contains(out, "jackspeak") {
		t.Fatal("test premise: jackspeak should be cut without a query")
	}
	out := strings.Join(Compress(in, Options{MaxDepth: 5, Query: []string{"jackspeak"}}), "\n")
	for _, want := range []string{"jackspeak@3.4.3", "glob@10.5.0", "test-exclude@7.0.2"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q", want)
		}
	}
}

func TestFixtureKeepsPackages(t *testing.T) {
	out := strings.Join(Compress(fixture(t, "npm-ls-all.txt"), Options{MaxDepth: 5}), "\n")
	for _, want := range []string{"wrappy@1.0.2", "UNMET OPTIONAL DEPENDENCY jiti@*", "@webassemblyjs/leb128@1.13.2", "argparse@3.0.4 extraneous"} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q", want)
		}
	}
}

func TestGoModGraphFixture(t *testing.T) {
	in := fixture(t, "go-mod-graph.txt")
	out := Edges(in)
	if saved := 1 - float64(size(out))/float64(size(in)); saved < 0.3 {
		t.Errorf("saved %.0f%%", saved*100)
	}
	for _, l := range in {
		parent, child, _ := strings.Cut(l, " ")
		var ok bool
		for _, g := range out {
			if strings.HasPrefix(g, parent+": ") && strings.Contains(g, child) {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("edge %q lost", l)
		}
	}
}
