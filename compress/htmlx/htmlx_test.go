package htmlx

import (
	"strings"
	"testing"
)

func TestHeadingWithWrappedText(t *testing.T) {
	page := `<html><body><main><h2 id="hook-lifecycle"><div><a href="#hook-lifecycle">` + "​" + `</a></div><span>Hook lifecycle</span></h2><p>Runs at points.</p></main></body></html>`
	got, err := Markdown(page, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## Hook lifecycle\n\nRuns at points.") {
		t.Fatalf("got %q", got)
	}
}

func TestMainContentRoot(t *testing.T) {
	story := "<p>" + strings.Repeat("The harbour reopened after the storm. ", 20) + "</p>"
	tests := []struct {
		name, body string
		want, drop []string
	}{
		{
			name: "comment articles below the story",
			body: `<div class="post"><h1>Storm</h1>` + story + `</div><section><article class="comment">Nice post!</article></section>`,
			want: []string{"harbour reopened", "Nice post!"},
		},
		{
			name: "empty main before the real one",
			body: `<main class="l-main"></main><div>Menu Home About</div><main class="l-main">` + story + `</main>`,
			want: []string{"harbour reopened"},
			drop: []string{"Menu Home About"},
		},
		{
			name: "tiny photo article",
			body: `<article class="photo">Photo: harbour</article><div>` + story + `</div>`,
			want: []string{"harbour reopened"},
		},
		{
			name: "related posts box bigger than the story",
			body: `<article class="post">` + story + `</article><article class="related"><h3>You may like</h3>` +
				strings.Repeat(`<article><p>`+strings.Repeat("Another post teaser text here. ", 12)+`</p></article>`, 4) + `</article>`,
			want: []string{"harbour reopened"},
		},
		{
			name: "long comment outweighs a short story",
			body: `<article class="post"><p>Short story about the harbour reopened.</p>` +
				`<article class="comment-body"><p>` + strings.Repeat("I disagree with every word of this. ", 30) + `</p></article></article>`,
			want: []string{"harbour reopened"},
		},
		{
			name: "story article preferred",
			body: `<div>Subscribe now</div><article>` + story + `</article>`,
			want: []string{"harbour reopened"},
			drop: []string{"Subscribe now"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Markdown("<html><head><title>T</title></head><body>"+tt.body+"</body></html>", false)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, d := range tt.drop {
				if strings.Contains(got, d) {
					t.Errorf("kept %q in:\n%s", d, got)
				}
			}
		})
	}
}

func TestScriptLinksDropped(t *testing.T) {
	tests := []struct{ name, href string }{
		{"javascript", "javascript:void(0)"},
		{"mixed case", " JavaScript:alert(1)"},
		{"data", "data:text/html,x"},
		{"vbscript", "vbscript:x"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Markdown(`<body><p><a href="`+tc.href+`">label</a></p></body>`, true)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(got, "](") {
				t.Fatalf("link kept: %q", got)
			}
		})
	}
}
