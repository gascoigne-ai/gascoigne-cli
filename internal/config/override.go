package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/internal/router"
)

// Overrides adjust a config for one run, e.g. per agent, without editing files.
type Overrides struct {
	// Set holds "dotted.path=value" assignments; values are YAML
	// (optimizers.grep.max_per_file=50, router.default=[go,git]).
	Set []string
	// Only keeps just these optimizers in every chain.
	Only []string
	// Disable drops these optimizers from every chain.
	Disable []string
}

// ParseWith parses data after applying overrides.
func ParseWith(data []byte, o Overrides) (*Config, error) {
	data, err := applySets(data, o.Set)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	c, err := Parse(data)
	if err != nil {
		return nil, err
	}
	if err := c.restrict(o.Only, o.Disable); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

func applySets(data []byte, sets []string) ([]byte, error) {
	if len(sets) == 0 {
		return data, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	for _, s := range sets {
		path, value, ok := strings.Cut(s, "=")
		if !ok || path == "" {
			return nil, fmt.Errorf("set %q: want path=value", s)
		}
		var v yaml.Node
		if err := yaml.Unmarshal([]byte(value), &v); err != nil {
			return nil, fmt.Errorf("set %s: %w", path, err)
		}
		node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}
		if len(v.Content) > 0 {
			node = v.Content[0]
		}
		if err := setPath(doc.Content[0], strings.Split(path, "."), node); err != nil {
			return nil, fmt.Errorf("set %s: %w", path, err)
		}
	}
	return yaml.Marshal(&doc)
}

// setPath assigns v at keys inside mapping n, creating mappings on the way.
func setPath(n *yaml.Node, keys []string, v *yaml.Node) error {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		n.Kind, n.Tag, n.Value = yaml.MappingNode, "!!map", ""
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("%q is not a mapping", keys[0])
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value != keys[0] {
			continue
		}
		if len(keys) == 1 {
			n.Content[i+1] = v
			return nil
		}
		return setPath(n.Content[i+1], keys[1:], v)
	}
	child := v
	if len(keys) > 1 {
		child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: keys[0]}, child)
	if len(keys) > 1 {
		return setPath(child, keys[1:], v)
	}
	return nil
}

// restrict filters optimizer chains of all routers.
func (c *Config) restrict(only, disable []string) error {
	if len(only) == 0 && len(disable) == 0 {
		return nil
	}
	var errs []error
	for _, id := range slices.Concat(only, disable) {
		if _, ok := c.Optimizers[id]; !ok {
			errs = append(errs, fmt.Errorf("optimizer %q not defined", id))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	keep := func(id string) bool {
		return (len(only) == 0 || slices.Contains(only, id)) && !slices.Contains(disable, id)
	}
	filter := func(r *router.Config) {
		r.Default = slices.DeleteFunc(r.Default, func(id string) bool { return !keep(id) })
		for i := range r.Routes {
			r.Routes[i].Optimizers = slices.DeleteFunc(r.Routes[i].Optimizers, func(id string) bool { return !keep(id) })
		}
	}
	filter(&c.Router)
	for _, p := range c.Pipelines {
		if p.Router != nil {
			filter(p.Router)
		}
	}
	return nil
}
