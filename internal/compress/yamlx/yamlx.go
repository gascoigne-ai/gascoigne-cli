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
func block(node *yaml.Node) {
	node.Style &^= yaml.FlowStyle
	if node.Kind == yaml.ScalarNode && node.Style&yaml.DoubleQuotedStyle != 0 && plain(node.Value) {
		node.Style &^= yaml.DoubleQuotedStyle
	}
	for _, c := range node.Content {
		block(c)
	}
}

// plain reports strings safe without quotes (no YAML-special meaning).
func plain(text string) bool {
	if text == "" || strings.TrimSpace(text) != text || strings.ContainsAny(text, "\n{}[]") ||
		strings.ContainsAny(text[:1], "-?:,#&*!|>'\"%@`") || strings.Contains(text, ": ") ||
		strings.Contains(text, " #") || strings.HasSuffix(text, ":") {
		return false
	}
	switch strings.ToLower(text) {
	case "true", "false", "yes", "no", "on", "off", "null", "~":
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			if r != '.' && r != '-' && r != '+' && r != 'e' && r != 'E' {
				return true
			}
		}
	}
	return false // numeric-looking: keep quotes
}

// Prune applies rules and then drops empty mappings, sequences and nulls.
func Prune(node *yaml.Node, rules []Rule) {
	prune(node, "", rules)
}

func prune(node *yaml.Node, parent string, rules []Rule) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, c := range node.Content {
			prune(c, parent, rules)
		}
	case yaml.SequenceNode:
		var keep []*yaml.Node
		for _, item := range node.Content {
			if dropItem(item, parent, rules) {
				continue
			}
			prune(item, parent, rules)
			if !empty(item) {
				keep = append(keep, item)
			}
		}
		node.Content = keep
	case yaml.MappingNode:
		var keep []*yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			k, v := node.Content[i], node.Content[i+1]
			if dropEntry(k.Value, v, parent, rules) {
				continue
			}
			prune(v, k.Value, rules)
			if empty(v) {
				continue
			}
			keep = append(keep, k, v)
		}
		node.Content = keep
	}
}

func dropEntry(key string, value *yaml.Node, parent string, rules []Rule) bool {
	for _, r := range rules {
		if r.Key != key || r.ItemField != "" || r.ItemMatch != nil || r.Parent != "" && r.Parent != parent {
			continue
		}
		if r.Equals == "" || value.Kind == yaml.ScalarNode && value.Value == r.Equals {
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

func field(mapping *yaml.Node, key string) string {
	if mapping.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1].Value
		}
	}
	return "\x00" // absent: never equals a wanted value
}

func empty(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.MappingNode, yaml.SequenceNode:
		return len(node.Content) == 0
	case yaml.ScalarNode:
		return node.Tag == "!!null" || node.Value == "" && node.Style == 0
	}
	return false
}
