// Package schema compacts the JSON Schemas of tool definitions without
// changing what they allow: agents send every tool's schema with every
// request, and generated schemas say some things twice — titles restating a
// property's name, "$schema" and "$id" markers. Constraints the API checks
// (additionalProperties, required, types) are never touched.
package schema

import (
	"strings"
	"unicode"
)

// meta are keys that describe the document, not the input.
var meta = []string{"$schema", "$id", "$comment"}

// suffixes generated root titles add to a tool's name ("read_fileArguments").
var suffixes = []string{"", "arguments", "args", "input", "inputs", "params", "parameters", "request", "schema"}

// Compact removes redundant keys from tool's input schema in place and
// reports whether it changed anything.
func Compact(tool string, node map[string]any) bool {
	changed := dropTitle(node, func(title string) bool {
		for _, suf := range suffixes {
			if title == fold(tool)+suf {
				return true
			}
		}
		return false
	})
	return walk(node) || changed
}

// walk drops meta keys and restating titles below s.
func walk(node map[string]any) bool {
	changed := false
	for _, k := range meta {
		if _, ok := node[k]; ok {
			delete(node, k)
			changed = true
		}
	}
	for _, key := range []string{"properties", "$defs", "definitions", "patternProperties"} {
		named, _ := node[key].(map[string]any)
		for name, v := range named {
			sub, ok := v.(map[string]any)
			if !ok {
				continue
			}
			if dropTitle(sub, func(title string) bool { return title == fold(name) }) {
				changed = true
			}
			if walk(sub) {
				changed = true
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties", "not", "if", "then", "else", "contains"} {
		if sub, ok := node[key].(map[string]any); ok && walk(sub) {
			changed = true
		}
	}
	for _, key := range []string{"anyOf", "oneOf", "allOf", "prefixItems", "items"} {
		list, _ := node[key].([]any)
		for _, v := range list {
			if sub, ok := v.(map[string]any); ok && walk(sub) {
				changed = true
			}
		}
	}
	return changed
}

// dropTitle removes s's title when same reports its folded form redundant.
func dropTitle(node map[string]any, same func(string) bool) bool {
	t, ok := node["title"].(string)
	if !ok || !same(fold(t)) {
		return false
	}
	delete(node, "title")
	return true
}

// fold lowercases and keeps letters and digits: "File Path", "file_path"
// and "filePath" all read "filepath".
func fold(text string) string {
	return strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			return unicode.ToLower(char)
		}
		return -1
	}, text)
}
