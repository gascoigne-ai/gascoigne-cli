package htmlx

import (
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Link-dense blocks: mega-menus, tag clouds and "popular" lists written as
// plain lists of links, not inside <nav>. They fill the budget before the story
// does. A block counts when it is made of links: many of them, short labels,
// almost no text outside them. They are dropped like <nav> is, and only when the
// page has plenty of other text, so a page that is itself a list of links
// (a table of contents, an awesome-list) keeps it. The original stays in the
// ledger like any optimized page.
const (
	denseMinLinks   = 6    // fewer links is a sentence with links in it
	denseLinkShare  = 85   // percent of the block's text that sits inside links
	denseMaxLabel   = 50   // mean label length: longer ones are titles worth reading
	denseMinOutside = 1500 // text bytes the page must have outside such blocks
)

// denseBlocks finds the outermost link-dense blocks under root, provided the
// page keeps enough text outside them.
func denseBlocks(root *html.Node) map[*html.Node]bool {
	found := map[*html.Node]bool{}
	dropped := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skip[n.DataAtom] {
			return
		}
		switch n.DataAtom { // a document or text node has none of these
		case atom.Ul, atom.Ol, atom.Div, atom.Section, atom.Dl, atom.P, atom.Span:
			if links, linkText, all := linkStats(n); linkDense(links, linkText, all) {
				found[n] = true
				dropped += all
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if len(found) == 0 || visible(root)-dropped < denseMinOutside {
		return nil
	}
	return found
}

func linkDense(links, linkText, all int) bool {
	return links >= denseMinLinks && all > 0 && 100*linkText >= denseLinkShare*all && linkText/links <= denseMaxLabel
}

// linkStats counts the links under n, the text bytes inside them and all the
// visible text bytes.
func linkStats(n *html.Node) (links, linkText, all int) {
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, inLink bool) {
		switch {
		case n.Type == html.TextNode:
			t := len(trimmed(n.Data))
			all += t
			if inLink {
				linkText += t
			}
			return
		case n.Type != html.ElementNode || skip[n.DataAtom]:
			return
		case n.DataAtom == atom.A && attr(n, "href") != "" && !inLink:
			links++
			inLink = true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inLink)
		}
	}
	walk(n, false)
	return links, linkText, all
}
