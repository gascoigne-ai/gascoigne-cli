package configs

import "go.yaml.in/yaml/v3"

// Overlay lays over on base: maps merge key by key, anything else (values,
// lists) replaces. Keys left empty (a parent uncommented without children)
// keep the base value.
func Overlay(base, over []byte) ([]byte, error) {
	var b, o yaml.Node
	if err := yaml.Unmarshal(base, &b); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(over, &o); err != nil {
		return nil, err
	}
	if len(o.Content) == 0 || len(b.Content) == 0 {
		return base, nil
	}
	merge(b.Content[0], o.Content[0])
	Plain(&b)
	return yaml.Marshal(&b)
}

// Plain drops comments and flow style: yaml.v3 writes broken YAML for
// comments inside a flow mapping that gained keys, and nobody reads the
// merged document but the decoder.
func Plain(node *yaml.Node) {
	node.HeadComment, node.LineComment, node.FootComment = "", "", ""
	if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
		node.Style = 0
	}
	for _, c := range node.Content {
		Plain(c)
	}
}

func merge(dst, src *yaml.Node) {
	if src.Tag == "!!null" {
		return
	}
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		*dst = *src
		return
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		key, val := src.Content[i], src.Content[i+1]
		if j := KeyIndex(dst, key.Value); j >= 0 {
			merge(dst.Content[j+1], val)
		} else if val.Tag != "!!null" {
			dst.Content = append(dst.Content, key, val)
		}
	}
}

// KeyIndex is the index of key in mapping m, or -1.
func KeyIndex(node *yaml.Node, key string) int {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return i
		}
	}
	return -1
}
