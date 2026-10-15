package htmloptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestMarkdown(t *testing.T) {
	in := `<!doctype html><html><head><title>Doc</title><style>p{}</style></head><body>
<nav><a href="/">Home</a></nav>
<main>
  <h1>Doc</h1>
  <p>Hello <a href="https://x.io">world</a>, see <code>f()</code>.
     More   text.</p>
  <ul><li>one</li><li>two<ol><li>a</li></ol></li></ul>
  <pre>x := 1
y := 2</pre>
  <script>track()</script>
  <table><tr><th>k</th><th>v</th></tr><tr><td>a</td><td>1</td></tr></table>
</main>
<footer>(c) corp</footer>
</body></html>`
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.HTML}}}
	if err := (&Optimizer{cfg: &Config{Links: true}}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	want := "# Doc\n\nHello [world](https://x.io), see `f()`. More text.\n\n- one\n- two\n\n  1. a\n\n```\nx := 1\ny := 2\n```\n\n| k | v |\n| a | 1 |\n"
	if got := p.Blocks[0].Text; got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if p.Blocks[0].Type != detect.Markdown {
		t.Fatalf("type %q", p.Blocks[0].Type)
	}
}
