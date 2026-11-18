package htmlx

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// writer renders a node tree as markdown.
type writer struct {
	b      strings.Builder
	links  bool
	lists  []list
	quote  int
	inPre  bool
	inLink bool
	dense  map[*html.Node]bool // link-dense blocks to leave out
}

type list struct {
	ordered bool
	n       int
}

// elements renders one element kind; atoms not listed render their children.
var elements = map[atom.Atom]func(md *writer, node *html.Node){}

func init() {
	for a, marks := range map[atom.Atom]string{atom.H1: "#", atom.H2: "##", atom.H3: "###", atom.H4: "####", atom.H5: "#####", atom.H6: "######"} {
		elements[a] = func(md *writer, node *html.Node) {
			md.block()
			md.b.WriteString(marks + " " + md.inline(node))
			md.block()
		}
	}
	for _, a := range []atom.Atom{atom.P, atom.Div, atom.Section, atom.Article, atom.Main, atom.Dl, atom.Figure, atom.Details, atom.Summary} {
		elements[a] = func(md *writer, node *html.Node) {
			md.block()
			md.children(node)
			md.block()
		}
	}
	for _, a := range []atom.Atom{atom.Dt, atom.Dd} {
		elements[a] = func(md *writer, node *html.Node) {
			md.line()
			md.children(node)
		}
	}
	for _, a := range []atom.Atom{atom.Ul, atom.Ol} {
		elements[a] = func(md *writer, node *html.Node) {
			md.lists = append(md.lists, list{ordered: a == atom.Ol})
			md.block()
			md.children(node)
			md.lists = md.lists[:len(md.lists)-1]
			md.block()
		}
	}
	for _, a := range []atom.Atom{atom.Td, atom.Th} {
		elements[a] = func(md *writer, node *html.Node) {
			md.b.WriteString("| ")
			md.children(node)
			md.b.WriteString(" ")
		}
	}
	elements[atom.Tr] = func(md *writer, node *html.Node) {
		md.line()
		md.children(node)
		md.b.WriteString("|")
	}
	elements[atom.Br] = func(md *writer, _ *html.Node) { md.line() }
	elements[atom.Hr] = func(md *writer, _ *html.Node) { md.block() }
	elements[atom.Li] = func(md *writer, node *html.Node) {
		md.item()
		md.children(node)
	}
	elements[atom.Pre] = func(md *writer, node *html.Node) {
		md.block()
		md.b.WriteString("```\n")
		md.inPre = true
		md.children(node)
		md.inPre = false
		if !strings.HasSuffix(md.b.String(), "\n") {
			md.b.WriteString("\n")
		}
		md.b.WriteString("```")
		md.block()
	}
	elements[atom.Code] = func(md *writer, node *html.Node) {
		if md.inPre {
			md.children(node)
			return
		}
		md.b.WriteString("`")
		md.children(node)
		md.b.WriteString("`")
	}
	elements[atom.A] = (*writer).link
	elements[atom.Blockquote] = func(md *writer, node *html.Node) {
		md.quote++
		md.block()
		md.children(node)
		md.quote--
		md.block()
	}
}

func (w *writer) node(node *html.Node) {
	switch node.Type {
	case html.TextNode:
		w.text(node.Data)
		return
	case html.ElementNode:
	default:
		w.children(node)
		return
	}
	if skip[node.DataAtom] || w.dense[node] || attr(node, "hidden") != "" || attr(node, "aria-hidden") == "true" {
		return
	}
	if render, ok := elements[node.DataAtom]; ok {
		render(w, node)
		return
	}
	w.children(node)
}

func (w *writer) children(node *html.Node) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		w.node(c)
	}
}

func (w *writer) text(text string) {
	if w.inPre {
		w.b.WriteString(text)
		return
	}
	text = spaces.ReplaceAllString(text, " ")
	if w.atLineStart() {
		text = strings.TrimLeft(text, " ")
	}
	w.b.WriteString(text)
}

// scriptScheme reports hrefs that run code or embed data instead of pointing at a page.
func scriptScheme(href string) bool {
	h := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(h, "javascript:") || strings.HasPrefix(h, "data:") || strings.HasPrefix(h, "vbscript:")
}

func (w *writer) link(node *html.Node) {
	href := attr(node, "href")
	if !w.links || w.inLink || href == "" || strings.HasPrefix(href, "#") || scriptScheme(href) {
		w.children(node)
		return
	}
	start := w.b.Len()
	w.inLink = true
	w.children(node)
	w.inLink = false
	label := strings.TrimSpace(w.b.String()[start:])
	if label == "" {
		return
	}
	out := w.b.String()[:start] + "[" + label + "](" + href + ")"
	w.b.Reset()
	w.b.WriteString(out)
}

func (w *writer) atLineStart() bool {
	s := w.b.String()
	return s == "" || strings.HasSuffix(s, "\n") || strings.HasSuffix(s, "- ") || strings.HasSuffix(s, "> ")
}

// block ends the current paragraph.
// inline renders n's children on one line: doc sites wrap heading text in
// divs and anchor icons ("<h2><div><a>\u200b</a></div><span>Hooks</span>"),
// which would otherwise leave an empty "##" with the title lines below.
func (w *writer) inline(node *html.Node) string {
	sub := &writer{links: w.links}
	sub.children(node)
	text := strings.Map(func(char rune) rune {
		if char == '\u200b' || char == '\ufeff' {
			return -1
		}
		return char
	}, sub.b.String())
	return strings.Join(strings.Fields(text), " ")
}

func (w *writer) block() {
	w.trimTrailingSpace()
	if w.b.Len() > 0 && !strings.HasSuffix(w.b.String(), "\n\n") {
		w.b.WriteString("\n\n")
	}
	w.prefix()
}

func (w *writer) line() {
	w.trimTrailingSpace()
	w.b.WriteString("\n")
	w.prefix()
}

func (w *writer) item() {
	w.trimTrailingSpace()
	if !strings.HasSuffix(w.b.String(), "\n") {
		w.b.WriteString("\n")
	}
	depth := max(len(w.lists)-1, 0)
	w.b.WriteString(strings.Repeat("  ", depth))
	if len(w.lists) > 0 && w.lists[len(w.lists)-1].ordered {
		l := &w.lists[len(w.lists)-1]
		l.n++
		w.b.WriteString(strconv.Itoa(l.n) + ". ")
		return
	}
	w.b.WriteString("- ")
}

func (w *writer) prefix() {
	if w.quote > 0 {
		w.b.WriteString(strings.Repeat("> ", w.quote))
	}
}

func (w *writer) trimTrailingSpace() {
	s := w.b.String()
	if t := strings.TrimRight(s, " \t"); len(t) != len(s) {
		w.b.Reset()
		w.b.WriteString(t)
	}
}
