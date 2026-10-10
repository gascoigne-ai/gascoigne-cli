// Package yamlx prunes YAML (and JSON, a YAML subset) documents: keys that
// carry no information for a reader (bookkeeping, defaults, empty values)
// are dropped while key order and structure stay.
package yamlx

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Rule drops a mapping entry or a sequence item.
type Rule struct {
	// Key drops entries with this key (at any depth unless Parent is set).
	Key string
	// Parent restricts Key to mappings found under this key ("metadata").
	Parent string
	// Equals drops the entry only when its scalar value equals it (defaults).
	Equals string
	// ItemField and ItemPrefix drop sequence items (under Key) whose field
	// starts with the prefix: volumes named kube-api-access-*.
	ItemField, ItemPrefix string
	// ItemMatch drops sequence items (under Key) whose fields all equal these.
	ItemMatch map[string]string
}

// Docs parses one or more YAML documents.
func Docs(text string) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	var docs []*yaml.Node
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &n)
	}
}

// Encode renders documents as YAML with 2-space indent, block style.
func Encode(docs []*yaml.Node) (string, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	for _, d := range docs {
		block(d)
		if err := enc.Encode(d); err != nil {
			return "", err
		}
	}
	err := enc.Close()
	return b.String(), err
}

// block turns JSON flow style into YAML block style (fewer tokens).
func block(n *yaml.Node) {
	n.Style &^= yaml.FlowStyle
	if n.Kind == yaml.ScalarNode && n.Style&yaml.DoubleQuotedStyle != 0 && plain(n.Value) {
		n.Style &^= yaml.DoubleQuotedStyle
	}
	for _, c := range n.Content {
		block(c)
	}
}

// plain reports strings safe without quotes (no YAML-special meaning).
func plain(s string) bool {
	if s == "" || strings.TrimSpace(s) != s || strings.ContainsAny(s, "\n{}[]") ||
		strings.ContainsAny(s[:1], "-?:,#&*!|>'\"%@`") || strings.Contains(s, ": ") ||
		strings.Contains(s, " #") || strings.HasSuffix(s, ":") {
		return false
	}
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			if r != '.' && r != '-' && r != '+' && r != 'e' && r != 'E' {
				return true
			}
		}
	}
	return false // numeric-looking: keep quotes
}

// Prune applies rules and then drops empty mappings, sequences and nulls.
func Prune(n *yaml.Node, rules []Rule) {
	prune(n, "", rules)
}

func prune(n *yaml.Node, parent string, rules []Rule) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			prune(c, parent, rules)
		}
	case yaml.SequenceNode:
		var keep []*yaml.Node
		for _, item := range n.Content {
			if dropItem(item, parent, rules) {
				continue
			}
			prune(item, parent, rules)
			if !empty(item) {
				keep = append(keep, item)
			}
		}
		n.Content = keep
	case yaml.MappingNode:
		var keep []*yaml.Node
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if dropEntry(k.Value, v, parent, rules) {
				continue
			}
			prune(v, k.Value, rules)
			if empty(v) {
				continue
			}
			keep = append(keep, k, v)
		}
		n.Content = keep
	}
}

func dropEntry(key string, v *yaml.Node, parent string, rules []Rule) bool {
	for _, r := range rules {
		if r.Key != key || r.ItemField != "" || r.ItemMatch != nil || r.Parent != "" && r.Parent != parent {
			continue
		}
		if r.Equals == "" || v.Kind == yaml.ScalarNode && v.Value == r.Equals {
			return true
		}
	}
	return false
}

func dropItem(item *yaml.Node, parent string, rules []Rule) bool {
	for _, r := range rules {
		if r.Key != parent {
			continue
		}
		if r.ItemField != "" && strings.HasPrefix(field(item, r.ItemField), r.ItemPrefix) {
			return true
		}
		if r.ItemMatch != nil {
			match := true
			for f, want := range r.ItemMatch {
				if field(item, f) != want {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func field(m *yaml.Node, key string) string {
	if m.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1].Value
		}
	}
	return "\x00" // absent: never equals a wanted value
}

func empty(n *yaml.Node) bool {
	switch n.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return len(n.Content) == 0
	case yaml.ScalarNode:
		return n.Tag == "!!null" || n.Value == "" && n.Style == 0
	}
	return false
}
