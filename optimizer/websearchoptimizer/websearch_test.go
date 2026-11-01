package websearchoptimizer

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
			name: "links as lines, host-only titles and repeated urls dropped",
			in: `Web search results for query: "x"

Links: [{"title":"plugin api","url":"https://www.a.test/sdk"},{"title":"b.test","url":"https://b.test/p"},{"title":"again","url":"https://b.test/p"}]

Summary stays.

REMINDER: cite.
`,
			want: `Web search results for query: "x"

Links:
- plugin api: https://www.a.test/sdk
- https://b.test/p

Summary stays.

REMINDER: cite.
`,
		},
		{
			name:  "over the cap the query's match, then the engine's first, stay in engine order",
			query: "gamma",
			in:    "Links: " + linksJSON(5, 3) + "\n",
			want:  "Links:\n- t0: https://s.test/0\n- t1: https://s.test/1\n- t3 gamma: https://s.test/3\n- … 2 more links dropped\n",
		},
		{
			name: "several searches shrink each Links line",
			in:   "Links: [{\"title\":\"a\",\"url\":\"https://a.test\"}]\ntext\nLinks: [{\"title\":\"b\",\"url\":\"https://b.test\"}]\n",
			want: "Links:\n- a: https://a.test\ntext\nLinks:\n- b: https://b.test\n",
		},
		{
			name: "unparsable or url-less links stay as written",
			in:   "Links: [{\"title\":\"a\"}]\nLinks: [oops\n",
			want: "Links: [{\"title\":\"a\"}]\nLinks: [oops\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &Optimizer{cfg: &Config{MaxLinks: 3}}
			if got := o.shrink(tt.in, sample.Terms(tt.query)); got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// linksJSON is n links whose title t<i> only matches the query when i == hit.
func linksJSON(n, hit int) string {
	var items []string
	for i := range n {
		title := fmt.Sprintf("t%d", i)
		if i == hit {
			title += " gamma"
		}
		items = append(items, fmt.Sprintf(`{"title":%q,"url":"https://s.test/%d"}`, title, i))
	}
	return "[" + strings.Join(items, ",") + "]"
}
