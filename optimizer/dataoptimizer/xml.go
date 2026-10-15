package dataoptimizer

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

type node struct {
	name     string
	attrs    []string // name="value"
	text     string
	children []*node
	omitted  int // siblings dropped before this node
}

// xml re-indents the tree compactly without comments, namespace declarations
// and schema locations, and keeps the ends of long runs of same-named
// siblings plus every one holding a failure (JUnit reports, SOAP faults).
func (o *Optimizer) xml(text string) (string, error) {
	root, err := parseXML(text)
	if err != nil {
		return "", err
	}
	o.sample(root)
	var b strings.Builder
	for _, c := range root.children {
		write(&b, c, 0)
	}
	return b.String(), nil
}

func parseXML(text string) (*node, error) {
	dec := xml.NewDecoder(strings.NewReader(text))
	dec.Strict = false
	root := &node{}
	stack := []*node{root}
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: qname(t.Name)}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Local == "schemaLocation" || a.Name.Local == "noNamespaceSchemaLocation" {
					continue
				}
				n.attrs = append(n.attrs, fmt.Sprintf("%s=%q", qname(a.Name), a.Value))
			}
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if s := strings.TrimSpace(string(t)); s != "" {
				top.text = strings.TrimSpace(top.text + " " + s)
			}
		}
	}
}

func qname(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

var failed = lazyre.New(`^(\w+:)?(failure|error|fault|Fault)$|(status|result|outcome)="(fail|failed|error|Failed|Error)"`)

func (n *node) failed() bool {
	if failed.MatchString(n.name) {
		return true
	}
	for _, a := range n.attrs {
		if failed.MatchString(a) {
			return true
		}
	}
	for _, c := range n.children {
		if c.failed() {
			return true
		}
	}
	return false
}

// sample keeps head and tail of each name's siblings past MaxItems, and
// failures anywhere.
func (o *Optimizer) sample(n *node) {
	count := map[string]int{}
	for _, c := range n.children {
		count[c.name]++
	}
	seen := map[string]int{}
	var kept []*node
	dropped := map[string]int{}
	head, tail := o.cfg.MaxItems*2/3, o.cfg.MaxItems/3
	for _, c := range n.children {
		i, total := seen[c.name], count[c.name]
		seen[c.name]++
		if total > o.cfg.MaxItems && i >= head && i < total-tail && !c.failed() {
			dropped[c.name]++
			continue
		}
		c.omitted, dropped[c.name] = dropped[c.name], 0
		kept = append(kept, c)
		o.sample(c)
	}
	n.children = kept
}

func write(b *strings.Builder, n *node, depth int) {
	pad := strings.Repeat(" ", depth)
	if n.omitted > 0 {
		fmt.Fprintf(b, "%s<!-- %s -->\n", pad, compress.Omitted(n.omitted, "<"+n.name+"> elements"))
	}
	open := n.name
	if len(n.attrs) > 0 {
		open += " " + strings.Join(n.attrs, " ")
	}
	switch {
	case len(n.children) == 0 && n.text == "":
		fmt.Fprintf(b, "%s<%s/>\n", pad, open)
	case len(n.children) == 0:
		fmt.Fprintf(b, "%s<%s>%s</%s>\n", pad, open, escape(n.text), n.name)
	default:
		fmt.Fprintf(b, "%s<%s>\n", pad, open)
		if n.text != "" {
			fmt.Fprintf(b, "%s %s\n", pad, escape(n.text))
		}
		for _, c := range n.children {
			write(b, c, depth+1)
		}
		fmt.Fprintf(b, "%s</%s>\n", pad, n.name)
	}
}

// escape keeps newlines (stack traces in failure bodies).
var escape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
