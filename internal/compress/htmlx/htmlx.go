// Package htmlx turns web pages into markdown: scripts, styles, navigation,
// forms and other chrome dropped, main content preferred.
package htmlx

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
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
	root := mainContent(doc)
	w := &writer{links: links, dense: denseBlocks(root)}
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
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.DataAtom == atom.Main || node.DataAtom == atom.Article) && !holdsArticle(node) && !isComment(node) {
			if l := visible(node); l > bestLen {
				best, bestLen = node, l
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if best == nil || 2*bestLen < visible(doc) {
		return doc
	}
	return best
}

// trimmed is s without surrounding space.
func trimmed(text string) string { return strings.TrimSpace(text) }

func isComment(node *html.Node) bool {
	return strings.Contains(strings.ToLower(attr(node, "class")+" "+attr(node, "id")), "comment")
}

func holdsArticle(node *html.Node) bool {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if find(c, atom.Article) != nil {
			return true
		}
	}
	return false
}

// visible counts the non-space text bytes the writer would keep.
func visible(node *html.Node) int {
	if node.Type == html.ElementNode && skip[node.DataAtom] {
		return 0
	}
	if node.Type == html.TextNode {
		return len(strings.TrimSpace(node.Data))
	}
	total := 0
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		total += visible(c)
	}
	return total
}

func find(node *html.Node, tag atom.Atom) *html.Node {
	if node.Type == html.ElementNode && node.DataAtom == tag {
		return node
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if f := find(c, tag); f != nil {
			return f
		}
	}
	return nil
}

func text(node *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return b.String()
}

func attr(node *html.Node, key string) string {
	for _, a := range node.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
