// Package htmloptimizer turns web pages into markdown: scripts, styles,
// navigation, forms and other chrome dropped, main content preferred.
package htmloptimizer

import (
	"context"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if b.Type != detect.HTML {
			continue
		}
		doc, err := html.Parse(strings.NewReader(b.Text))
		if err != nil {
			return err
		}
		p.Blocks[i].Text = o.markdown(doc)
		p.Blocks[i].Type = detect.Markdown
	}
	return nil
}

var skip = map[atom.Atom]bool{
	atom.Script: true, atom.Style: true, atom.Noscript: true, atom.Svg: true, atom.Nav: true,
	atom.Footer: true, atom.Header: true, atom.Aside: true, atom.Form: true, atom.Iframe: true,
	atom.Button: true, atom.Template: true, atom.Select: true, atom.Input: true, atom.Dialog: true,
	atom.Canvas: true, atom.Object: true, atom.Embed: true, atom.Img: true, atom.Picture: true,
	atom.Video: true, atom.Audio: true, atom.Head: true, atom.Label: true, atom.Menu: true,
}

func (o *Optimizer) markdown(doc *html.Node) string {
	w := &writer{links: o.cfg.Links}
	root := find(doc, atom.Main)
	if root == nil {
		root = find(doc, atom.Article)
	}
	if root == nil {
		root = doc
	}
	title := ""
	if t := find(doc, atom.Title); t != nil {
		title = strings.TrimSpace(text(t))
	}
	w.node(root)
	body := strings.TrimSpace(blankRuns.ReplaceAllString(w.b.String(), "\n\n"))
	if title != "" && !strings.Contains(body[:min(len(body), 400)], title) {
		body = "# " + title + "\n\n" + body
	}
	return body + "\n"
}

var (
	blankRuns = regexp.MustCompile(`\n[ \t]*\n(\s*\n)+`)
	spaces    = regexp.MustCompile(`\s+`)
)

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
