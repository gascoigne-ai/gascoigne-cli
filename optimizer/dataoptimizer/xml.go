package dataoptimizer

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
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
			n := newElement(t)
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

// newElement is the node of a start tag; namespace declarations and schema
// locations are noise and not kept as attributes.
func newElement(start xml.StartElement) *node {
	n := &node{name: qname(start.Name)}
	for _, a := range start.Attr {
		if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" || a.Name.Local == "schemaLocation" || a.Name.Local == "noNamespaceSchemaLocation" {
			continue
		}
		n.attrs = append(n.attrs, fmt.Sprintf("%s=%q", qname(a.Name), a.Value))
	}
	return n
}

func qname(name xml.Name) string {
	if name.Space != "" {
		return name.Space + ":" + name.Local
	}
	return name.Local
}

var failed = lazyre.New(`^(\w+:)?(failure|error|fault|Fault)$|(status|result|outcome)="(fail|failed|error|Failed|Error)"`)

func (n *node) failed() bool {
	if failed.MatchString(n.name) {
		return true
	}
	if slices.ContainsFunc(n.attrs, failed.MatchString) {
		return true
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
func (o *Optimizer) sample(element *node) {
	count := map[string]int{}
	for _, c := range element.children {
		count[c.name]++
	}
	seen := map[string]int{}
	var kept []*node
	dropped := map[string]int{}
	head, tail := o.cfg.MaxItems*2/3, o.cfg.MaxItems/3
	for _, c := range element.children {
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
	element.children = kept
}

func write(out *strings.Builder, element *node, depth int) {
	pad := strings.Repeat(" ", depth)
	if element.omitted > 0 {
		fmt.Fprintf(out, "%s<!-- %s -->\n", pad, compress.Omitted(element.omitted, "<"+element.name+"> elements"))
	}
	open := element.name
	if len(element.attrs) > 0 {
		open += " " + strings.Join(element.attrs, " ")
	}
	switch {
	case len(element.children) == 0 && element.text == "":
		fmt.Fprintf(out, "%s<%s/>\n", pad, open)
	case len(element.children) == 0:
		fmt.Fprintf(out, "%s<%s>%s</%s>\n", pad, open, escape(element.text), element.name)
	default:
		fmt.Fprintf(out, "%s<%s>\n", pad, open)
		if element.text != "" {
			fmt.Fprintf(out, "%s %s\n", pad, escape(element.text))
		}
		for _, c := range element.children {
			write(out, c, depth+1)
		}
		fmt.Fprintf(out, "%s</%s>\n", pad, element.name)
	}
}

// escape keeps newlines (stack traces in failure bodies).
var escape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
