package configs

import (
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// OpenKinds hold components by id: an id the defaults lack is a user's own
// instance (grep/strict), so only ids the defaults have get their fields checked.
var OpenKinds = []string{"extensions", "receivers", "filters", "optimizers", "exporters", "pipelines"}

// HasSettings reports whether a parsed config sets anything: a file of
// comments only, or one uncommented parent with nothing below, sets nothing.
func HasSettings(doc *yaml.Node) bool {
	if len(doc.Content) == 0 {
		return false
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return root.Tag != "!!null"
	}
	for i := 1; i < len(root.Content); i += 2 {
		if root.Content[i].Tag != "!!null" {
			return true
		}
	}
	return false
}

// Unknown lists the settings of a user config this version does
// not have, as dotted paths.
func Unknown(data []byte) []string {
	var user, defaults yaml.Node
	if yaml.Unmarshal(data, &user) != nil || yaml.Unmarshal(Default, &defaults) != nil || !HasSettings(&user) {
		return nil
	}
	return Prune(user.Content[0], defaults.Content[0], "")
}

// Prune removes from user the keys def does not have (recursing where both
// are maps) and returns the removed paths. Values and lists are leaves.
func Prune(user, def *yaml.Node, path string) (dropped []string) {
	if user.Kind != yaml.MappingNode || def.Kind != yaml.MappingNode {
		return nil
	}
	var kept []*yaml.Node
	for i := 0; i+1 < len(user.Content); i += 2 {
		key, val := user.Content[i], user.Content[i+1]
		p := strings.TrimPrefix(path+"."+key.Value, ".")
		j := KeyIndex(def, key.Value)
		switch {
		case j >= 0:
			dropped = append(dropped, Prune(val, def.Content[j+1], p)...)
		case !slices.Contains(OpenKinds, path):
			dropped = append(dropped, p)
			continue
		}
		kept = append(kept, key, val)
	}
	user.Content = kept
	return dropped
}
