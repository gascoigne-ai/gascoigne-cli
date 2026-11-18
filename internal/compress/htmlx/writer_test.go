package htmlx

import "testing"

// TestWriterElements pins the markdown of every element the writer handles.
func TestWriterElements(t *testing.T) {
	page := `<html><body><main>
<h1>One</h1><h2>Two</h2><h3>Three</h3><h4>Four</h4><h5>Five</h5><h6>Six</h6>
<p>para <code>x := 1</code> and <a href="https://e.com/a">link</a><br>after break</p>
<div>div</div><section>sec</section><article>art</article><figure>fig</figure>
<details><summary>sum</summary>det</details>
<dl><dt>term</dt><dd>def</dd></dl>
<hr>
<ul><li>a</li><li>b<ol><li>x</li><li>y</li></ol></li></ul>
<ol><li>first</li><li>second</li></ol>
<pre><code>raw
  code</code></pre>
<blockquote>quoted<blockquote>deeper</blockquote></blockquote>
<table><tr><th>H1</th><th>H2</th></tr><tr><td>c1</td><td>c2</td></tr></table>
<span>span</span><em>em</em>
<script>skip()</script><p hidden>hid</p><p aria-hidden="true">aria</p>
</main></body></html>`
	got, err := Markdown(page, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "# One\n\n## Two\n\n### Three\n\n#### Four\n\n##### Five\n\n###### Six\n\npara `x := 1` and [link](https://e.com/a)\nafter break\n\ndiv\n\nsec\n\nart\n\nfig\n\nsum\n\ndet\n\nterm\ndef\n\n- a\n- b\n\n  1. x\n  2. y\n\n1. first\n2. second\n\n```\nraw\n  code\n```\n\n> quoted\n\n> > deeper\n\n>\n\n| H1 | H2 |\n| c1 | c2 | spanem\n\nhid\n" {
		t.Errorf("got %q", got)
	}
}
