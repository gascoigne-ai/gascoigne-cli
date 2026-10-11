// Package htmlx turns web pages into markdown: scripts, styles, navigation,
// forms and other chrome dropped, main content preferred.
package htmlx

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lukaszb/gascoigne/lazyre"
)

var skip = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Svg: true, atom.Nav: true,
	atom.Footer: true, atom.Header: true, atom.Aside: true, atom.Form: true, atom.Iframe: true,
	atom.Button: true, atom.Template: true, atom.Select: true, atom.Input: true, atom.Dialog: true,
	atom.Canvas: true, atom.Object: true, atom.Embed: true, atom.Img: true, atom.Picture: true,
	atom.Video: true, atom.Audio: true, atom.Head: true, atom.Label: true, atom.Menu: true,
}

// Markdown converts a page: main content preferred, chrome dropped; links
// keeps hrefs as [text](url), otherwise only link text stays.
func Markdown(page string, links bool) (string, error) {
	doc, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return "", err
	}
	w := &writer{links: links}
	root := mainContent(doc)
	title := ""
	if t := find(doc, atom.Title); t != nil {
		title = strings.TrimSpace(text(t))
	}
	w.node(root)
	body := strings.TrimSpace(blankRuns.ReplaceAllString(w.b.String(), "\n\n"))
	if title != "" && !strings.Contains(body[:min(len(body), 400)], title) {
		body = "# " + title + "\n\n" + body
	}
	return body + "\n", nil
}

var (
	blankRuns = lazyre.New(`\n[ \t]*\n(\s*\n)+`)
	spaces    = lazyre.New(`\s+`)
)

// mainContent is the <main> or <article> holding the most visible text, when
// that is at least half the page's: the first such element is often empty,
// a comment or a photo box, and a page of many short articles (a listing)
// needs all of them. One holding other articles is a list (related posts),
// and one named a comment is a reader's, not the story. Else the whole page.
func mainContent(doc *html.Node) *html.Node {
	var best *html.Node
	bestLen := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.DataAtom == atom.Main || n.DataAtom == atom.Article) && !holdsArticle(n) && !isComment(n) {
			if l := visible(n); l > bestLen {
				best, bestLen = n, l
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if best == nil || 2*bestLen < visible(doc) {
		return doc
	}
	return best
}

func isComment(n *html.Node) bool {
	return strings.Contains(strings.ToLower(attr(n, "class")+" "+attr(n, "id")), "comment")
}

func holdsArticle(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if find(c, atom.Article) != nil {
			return true
		}
	}
	return false
}

// visible counts the non-space text bytes the writer would keep.
func visible(n *html.Node) int {
	if n.Type == html.ElementNode && skip[n.DataAtom] {
		return 0
	}
	if n.Type == html.TextNode {
		return len(strings.TrimSpace(n.Data))
	}
	total := 0
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		total += visible(c)
	}
	return total
}

func find(n *html.Node, a atom.Atom) *html.Node {
	if n.Type == html.ElementNode && n.DataAtom == a {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, a); f != nil {
			return f
		}
	}
	return nil
}

func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
