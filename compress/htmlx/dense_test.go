package htmlx

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

func links(n, labelLen int) string {
	var b strings.Builder
	for i := range n {
		fmt.Fprintf(&b, `<li><a href="/p/%d">%s%d</a></li>`, i, strings.Repeat("x", labelLen), i)
	}
	return b.String()
}

func TestLinkDenseBlocks(t *testing.T) {
	story := "<p>" + strings.Repeat("The harbour reopened after the storm. ", 60) + "</p>"
	tests := []struct {
		name, body string
		kept       bool // whether the link list survives
	}{
		{"a mega-menu written as a plain list is dropped", `<ul class="menu">` + links(14, 8) + `</ul>` + story, false},
		{"a tag cloud in a div is dropped", `<div class="tags">` + strings.Repeat(`<a href="/t">tag</a> `, 12) + `</div>` + story, false},
		{"links with long titles are worth reading", `<ul>` + links(10, 70) + `</ul>` + story, true},
		{"a few links are a sentence", `<ul>` + links(4, 8) + `</ul>` + story, true},
		{"text outside the links keeps the block", `<ul>` + strings.Repeat(`<li>Read <a href="/p">the post</a> about the harbour and the storm that closed it</li>`, 10) + `</ul>` + story, true},
		{"a page that is only a list of links keeps it", `<ul>` + links(30, 8) + `</ul>`, true},
		{"too little other text keeps it", `<ul>` + links(14, 8) + `</ul><p>A short note.</p>`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Markdown("<html><head><title>T</title></head><body><div>"+tt.body+"</div></body></html>", true)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, "harbour reopened") && strings.Contains(tt.body, "harbour reopened after") {
				t.Fatalf("the story was lost:\n%.300s", got)
			}
			has := strings.Contains(got, "](/p/0)") || strings.Contains(got, "[tag]") || strings.Contains(got, "the post")
			if has != tt.kept {
				t.Fatalf("link list kept = %v, want %v:\n%.400s", has, tt.kept, got)
			}
		})
	}
}

func TestLinkStats(t *testing.T) {
	doc := `<html><body><div><a href="/a">one</a> and <a href="/b">two</a><a>no href</a><a href="/c"><a href="/d">nested</a></a></div></body></html>`
	parsed, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	div := find(parsed, atom.Div)
	links, linkText, all := linkStats(div)
	// the parser closes an <a> that opens inside another: four links, one of them empty
	if links != 4 || linkText != len("one")+len("two")+len("nested") || all != linkText+len("and")+len("no href") {
		t.Fatalf("links %d, link text %d, all %d", links, linkText, all)
	}
}
