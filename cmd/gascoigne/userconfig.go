package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/configs"
)

// configData is the config to run: the built-in defaults with a config file
// laid over them (-config, $GASCOIGNE_CONFIG, else the user's). On first run
// the user's file is created from the commented template, so every setting
// is there to read and change while the defaults still apply.
func configData(path string) ([]byte, error) {
	path, own := configPath(path)
	if path == "" {
		return configs.Default, nil
	}
	if own {
		if err := writeUserConfig(path); err != nil {
			fmt.Fprintln(os.Stderr, "gascoigne: could not write a config template:", err)
			return configs.Default, nil
		}
	}
	over, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if own && !bytes.Equal(over, configs.User) {
		over = refreshTemplate(path, over)
		if bad := unknownSettings(over); len(bad) > 0 {
			return nil, fmt.Errorf("config %s: not settings in this version: %s; gascoigne config -upgrade drops them (old file kept as .bak)", path, strings.Join(bad, ", "))
		}
	}
	data, err := overlay(configs.Default, over)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return data, nil
}

// configExperimental is the config's experimental list. A config that
// does not load enables nothing; the commands that run it say why.
func configExperimental() []string {
	data, err := configData("")
	if err != nil {
		return nil
	}
	var c struct {
		Experimental []string `yaml:"experimental"`
	}
	if yaml.Unmarshal(data, &c) != nil {
		return nil
	}
	return c.Experimental
}

// configPath is the config file to read (-config, $GASCOIGNE_CONFIG, else
// the user's) and whether it is the user's own, which gascoigne writes and
// upgrades. "" when there is no user config dir.
func configPath(flag string) (path string, own bool) {
	if p := cmp.Or(flag, os.Getenv("GASCOIGNE_CONFIG")); p != "" {
		return p, false
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(dir, "gascoigne", "config.yaml"), true
}

// refreshTemplate brings a user config that sets nothing up to this
// version's template, so new settings show up in it. A file with settings is
// left alone: gascoigne config -upgrade moves those over.
func refreshTemplate(path string, data []byte) []byte {
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || hasSettings(&doc) {
		return data
	}
	if _, changed, err := writeUpgrade(path); err != nil || !changed {
		return data
	}
	fmt.Fprintf(os.Stderr, "gascoigne: updated %s to this version's settings (none of yours were set)\n", path)
	return configs.User
}

// writeUserConfig creates path from the template unless it exists.
func writeUserConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(configs.User); err != nil {
		f.Close()
		return err
	}
	fmt.Fprintf(os.Stderr, "gascoigne: wrote %s (every setting commented out: defaults apply)\n", path)
	return f.Close()
}

// overlay lays over on base: maps merge key by key, anything else (values,
// lists) replaces. Keys left empty (a parent uncommented without children)
// keep the base value.
func overlay(base, over []byte) ([]byte, error) {
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
	plain(&b)
	return yaml.Marshal(&b)
}

// plain drops comments and flow style: yaml.v3 writes broken YAML for
// comments inside a flow mapping that gained keys, and nobody reads the
// merged document but the decoder.
func plain(n *yaml.Node) {
	n.HeadComment, n.LineComment, n.FootComment = "", "", ""
	if n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode {
		n.Style = 0
	}
	for _, c := range n.Content {
		plain(c)
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
		if j := keyIndex(dst, key.Value); j >= 0 {
			merge(dst.Content[j+1], val)
		} else if val.Tag != "!!null" {
			dst.Content = append(dst.Content, key, val)
		}
	}
}

func keyIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}
