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
	switch a := node.DataAtom; {
	case headings[a] != "":
		w.block()
		w.b.WriteString(headings[a] + " " + w.inline(node))
		w.block()
	case a == atom.P || a == atom.Div || a == atom.Section || a == atom.Article || a == atom.Main ||
		a == atom.Dl || a == atom.Figure || a == atom.Details || a == atom.Summary:
		w.block()
		w.children(node)
		w.block()
	case a == atom.Dt || a == atom.Dd:
		w.line()
		w.children(node)
	case a == atom.Tr:
		w.line()
		w.children(node)
		w.b.WriteString("|")
	case a == atom.Br:
		w.line()
	case a == atom.Hr:
		w.block()
	case a == atom.Ul || a == atom.Ol:
		w.lists = append(w.lists, list{ordered: a == atom.Ol})
		w.block()
		w.children(node)
		w.lists = w.lists[:len(w.lists)-1]
		w.block()
	case a == atom.Li:
		w.item()
		w.children(node)
	case a == atom.Pre:
		w.block()
		w.b.WriteString("```\n")
		w.inPre = true
		w.children(node)
		w.inPre = false
		if !strings.HasSuffix(w.b.String(), "\n") {
			w.b.WriteString("\n")
		}
		w.b.WriteString("```")
		w.block()
	case a == atom.Code && !w.inPre:
		w.b.WriteString("`")
		w.children(node)
		w.b.WriteString("`")
	case a == atom.A:
		w.link(node)
	case a == atom.Blockquote:
		w.quote++
		w.block()
		w.children(node)
		w.quote--
		w.block()
	case a == atom.Td || a == atom.Th:
		w.b.WriteString("| ")
		w.children(node)
		w.b.WriteString(" ")
	default:
		w.children(node)
	}
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
