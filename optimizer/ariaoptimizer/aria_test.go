package ariaoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
)

func TestShrink(t *testing.T) {
	tests := []struct {
		name, query, in, want string
	}{
		{
			name: "wrappers unwrapped, pointer cursor dropped on links",
			in: `- Page URL: https://x.test/
- generic [ref=e1]:
  - generic [ref=e2]:
    - link "Home" [ref=e3] [cursor=pointer]:
      - /url: /
    - generic [ref=e4] [cursor=pointer]: Menu
`,
			want: `- Page URL: https://x.test/
- link "Home" [ref=e3]:
  - /url: /
- generic [ref=e4] [cursor=pointer]: Menu
`,
		},
		{
			name: "listitem around one node goes, around two stays",
			in: `- list [ref=e1]:
  - listitem [ref=e2]:
    - link "A" [ref=e3]
  - listitem [ref=e4]:
    - link "B" [ref=e5]
    - text: new
`,
			want: `- list [ref=e1]:
  - link "A" [ref=e3]
  - listitem [ref=e4]:
    - link "B" [ref=e5]
    - text: new
`,
		},
		{
			name:  "long run keeps head, tail and query matches",
			query: "item 12",
			in:    links(30),
			want: `- list [ref=e0]:
  - link "item 1" [ref=e1]
  - link "item 2" [ref=e2]
  - link "item 3" [ref=e3]
  - … 8 links omitted
  - link "item 12" [ref=e12]
  - … 16 links omitted
  - link "item 29" [ref=e29]
  - link "item 30" [ref=e30]
`,
		},
		{
			name: "text continuing a node stays with it",
			in: `- paragraph [ref=e1]:
    first line
    second line
- button "Go" [ref=e2]
`,
			want: `- paragraph [ref=e1]:
  first line
  second line
- button "Go" [ref=e2]
`,
		},
	}
	o := &Optimizer{cfg: &Config{MaxItems: 6, Head: 3, Tail: 2}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := o.shrink(tt.in, sample.Terms(tt.query)); got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func links(n int) string {
	var b strings.Builder
	b.WriteString("- list [ref=e0]:\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "  - link \"item %d\" [ref=e%d]\n", i, i)
	}
	return b.String()
}
