package main

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/configs"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
)

// keptHeader starts the settings an upgrade carried over from the old file.
const keptHeader = "## Your settings, kept by gascoigne config -upgrade (the previous file is next to this one, .bak)."

// openKinds hold components by id: an id the defaults lack is a user's own
// instance (grep/strict), so only ids the defaults have get their fields checked.
var openKinds = []string{"extensions", "receivers", "filters", "optimizers", "exporters", "pipelines"}

// upgradeConfig rebuilds a user config on the current template: the user's
// active settings are kept below it, settings this version no longer knows
// are left out and returned as dotted paths.
func upgradeConfig(old []byte) (out []byte, dropped []string, err error) {
	var user, defaults yaml.Node
	if err := yaml.Unmarshal(old, &user); err != nil {
		return nil, nil, fmt.Errorf("parse: %w", err)
	}
	if err := yaml.Unmarshal(configs.Default, &defaults); err != nil {
		return nil, nil, err
	}
	if !hasSettings(&user) {
		return configs.User, nil, nil
	}
	root := user.Content[0]
	plain(root) // the old template's comments would come back above the settings
	dropped = prune(root, defaults.Content[0], "")
	if len(root.Content) == 0 {
		return configs.User, dropped, nil
	}
	kept, err := yaml.Marshal(root)
	if err != nil {
		return nil, nil, err
	}
	merged, err := overlay(configs.Default, kept)
	if err != nil {
		return nil, nil, err
	}
	if _, err := config.Parse(merged); err != nil {
		return nil, nil, fmt.Errorf("kept settings do not load, fix them by hand: %w", err)
	}
	var b bytes.Buffer
	b.Write(configs.User)
	fmt.Fprintf(&b, "\n%s\n", keptHeader)
	b.Write(kept)
	return b.Bytes(), dropped, nil
}

// hasSettings reports whether a parsed config sets anything: a file of
// comments only, or one uncommented parent with nothing below, sets nothing.
func hasSettings(doc *yaml.Node) bool {
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

// unknownSettings lists the settings of a user config this version does
// not have, as dotted paths.
func unknownSettings(data []byte) []string {
	var user, defaults yaml.Node
	if yaml.Unmarshal(data, &user) != nil || yaml.Unmarshal(configs.Default, &defaults) != nil || !hasSettings(&user) {
		return nil
	}
	return prune(user.Content[0], defaults.Content[0], "")
}

// prune removes from user the keys def does not have (recursing where both
// are maps) and returns the removed paths. Values and lists are leaves.
func prune(user, def *yaml.Node, path string) (dropped []string) {
	if user.Kind != yaml.MappingNode || def.Kind != yaml.MappingNode {
		return nil
	}
	var kept []*yaml.Node
	for i := 0; i+1 < len(user.Content); i += 2 {
		key, val := user.Content[i], user.Content[i+1]
		p := strings.TrimPrefix(path+"."+key.Value, ".")
		j := keyIndex(def, key.Value)
		switch {
		case j >= 0:
			dropped = append(dropped, prune(val, def.Content[j+1], p)...)
		case !slices.Contains(openKinds, path):
			dropped = append(dropped, p)
			continue
		}
		kept = append(kept, key, val)
	}
	user.Content = kept
	return dropped
}

// writeUpgrade replaces path with the upgraded config, the old one kept as
// path.bak.
func writeUpgrade(path string) (dropped []string, changed bool, err error) {
	old, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	out, dropped, err := upgradeConfig(old)
	if err != nil {
		return nil, false, fmt.Errorf("upgrade %s: %w", path, err)
	}
	if bytes.Equal(out, old) {
		return nil, false, nil
	}
	if err := os.WriteFile(path+".bak", old, 0o644); err != nil {
		return nil, false, err
	}
	return dropped, true, os.WriteFile(path, out, 0o644)
}

// upgradeCmd runs gascoigne config -upgrade on the config file in use.
func upgradeCmd(flag string) error {
	path, _ := configPath(flag)
	if path == "" {
		return fmt.Errorf("config -upgrade: no config file (no user config dir)")
	}
	if !fileExists(path) {
		return fmt.Errorf("config -upgrade: %s does not exist", path)
	}
	dropped, changed, err := writeUpgrade(path)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Printf("gascoigne: %s is current\n", path)
		return nil
	}
	fmt.Printf("gascoigne: upgraded %s (previous file: %s.bak)\n", path, path)
	for _, p := range dropped {
		fmt.Printf("  dropped %s: not a setting in this version\n", p)
	}
	return nil
}
