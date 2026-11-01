package pkgoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// A depth-cut package comes back when the agent's query names it.
func TestTreeQuery(t *testing.T) {
	const in = `shop@1.0.0
├─┬ a@1
│ └─┬ b@1
│   └─┬ c@1
│     ├── needle@2.0.0
│     └── hay@1
└── z@1
`
	tests := []struct {
		name, query string
		cut         bool
	}{
		{"no query", "", true},
		{"other query", "hay stack", false},
		{"deep package", "needle", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &payload.Payload{
				Attrs:  map[string]string{payload.AttrQuery: tt.query},
				Blocks: []payload.Block{{Text: in, Type: "pkg/tree"}},
			}
			if err := (&Optimizer{cfg: &Config{TreeDepth: 2}}).Optimize(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			got := p.Blocks[0].Text
			if has := strings.Contains(got, "needle@2.0.0"); has == tt.cut {
				t.Errorf("needle shown = %v, want %v:\n%s", has, !tt.cut, got)
			}
		})
	}
}

func TestGoModGraph(t *testing.T) {
	p := &payload.Payload{Blocks: []payload.Block{{Text: "app a@v1\napp b@v2\na@v1 c@v3\n", Type: "pkg/gomodgraph"}}}
	if err := (&Optimizer{cfg: &Config{TreeDepth: 5}}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if got, want := p.Blocks[0].Text, "app: a@v1, b@v2\na@v1: c@v3"; strings.TrimSpace(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestValidate(t *testing.T) {
	if (&Config{TreeDepth: 0}).Validate() == nil {
		t.Error("tree_depth 0 accepted")
	}
}
