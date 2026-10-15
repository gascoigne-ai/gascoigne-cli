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

var headings = map[atom.Atom]string{atom.H1: "#", atom.H2: "##", atom.H3: "###", atom.H4: "####", atom.H5: "#####", atom.H6: "######"}

func (w *writer) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		w.text(n.Data)
		return
	case html.ElementNode:
	default:
		w.children(n)
		return
	}
	if skip[n.DataAtom] || w.dense[n] || attr(n, "hidden") != "" || attr(n, "aria-hidden") == "true" {
		return
	}
	switch a := n.DataAtom; {
	case headings[a] != "":
		w.block()
		w.b.WriteString(headings[a] + " " + w.inline(n))
		w.block()
	case a == atom.P || a == atom.Div || a == atom.Section || a == atom.Article || a == atom.Main ||
		a == atom.Dl || a == atom.Figure || a == atom.Details || a == atom.Summary:
		w.block()
		w.children(n)
		w.block()
	case a == atom.Dt || a == atom.Dd:
		w.line()
		w.children(n)
	case a == atom.Tr:
		w.line()
		w.children(n)
		w.b.WriteString("|")
	case a == atom.Br:
		w.line()
	case a == atom.Hr:
		w.block()
	case a == atom.Ul || a == atom.Ol:
		w.lists = append(w.lists, list{ordered: a == atom.Ol})
		w.block()
		w.children(n)
		w.lists = w.lists[:len(w.lists)-1]
		w.block()
	case a == atom.Li:
		w.item()
		w.children(n)
	case a == atom.Pre:
		w.block()
		w.b.WriteString("```\n")
		w.inPre = true
		w.children(n)
		w.inPre = false
		if !strings.HasSuffix(w.b.String(), "\n") {
			w.b.WriteString("\n")
		}
		w.b.WriteString("```")
		w.block()
	case a == atom.Code && !w.inPre:
		w.b.WriteString("`")
		w.children(n)
		w.b.WriteString("`")
	case a == atom.A:
		w.link(n)
	case a == atom.Blockquote:
		w.quote++
		w.block()
		w.children(n)
		w.quote--
		w.block()
	case a == atom.Td || a == atom.Th:
		w.b.WriteString("| ")
		w.children(n)
		w.b.WriteString(" ")
	default:
		w.children(n)
	}
}

func (w *writer) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c)
	}
}

func (w *writer) text(s string) {
	if w.inPre {
		w.b.WriteString(s)
		return
	}
	s = spaces.ReplaceAllString(s, " ")
	if w.atLineStart() {
		s = strings.TrimLeft(s, " ")
	}
	w.b.WriteString(s)
}

// scriptScheme reports hrefs that run code or embed data instead of pointing at a page.
func scriptScheme(href string) bool {
	h := strings.ToLower(strings.TrimSpace(href))
	return strings.HasPrefix(h, "javascript:") || strings.HasPrefix(h, "data:") || strings.HasPrefix(h, "vbscript:")
}

func (w *writer) link(n *html.Node) {
	href := attr(n, "href")
	if !w.links || w.inLink || href == "" || strings.HasPrefix(href, "#") || scriptScheme(href) {
		w.children(n)
		return
	}
	start := w.b.Len()
	w.inLink = true
	w.children(n)
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
func (w *writer) inline(n *html.Node) string {
	sub := &writer{links: w.links}
	sub.children(n)
	text := strings.Map(func(r rune) rune {
		if r == '\u200b' || r == '\ufeff' {
			return -1
		}
		return r
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
