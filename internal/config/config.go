// Package config loads and structurally validates the YAML config.
// Component-specific settings stay raw here; the service decodes them with factory defaults.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/router"
)

type Node = yaml.Node

type Components map[string]Node

type Config struct {
	Extensions Components          `yaml:"extensions"`
	Receivers  Components          `yaml:"receivers"`
	Filters    Components          `yaml:"filters"`
	Optimizers Components          `yaml:"optimizers"`
	Exporters  Components          `yaml:"exporters"`
	Router     router.Config       `yaml:"router"`
	Pipelines  map[string]Pipeline `yaml:"pipelines"`
	// Experimental names the CLI commands to enable that are not stable yet
	// ("all" for every one); GASCOIGNE_EXPERIMENTAL adds to it.
	Experimental []string `yaml:"experimental"`
	// Tokenizers configures exact token counts for gascoigne tokens.
	Tokenizers Tokenizers `yaml:"tokenizers"`
}

// Tokenizers sets how gascoigne tokens counts exactly, beside its estimates.
type Tokenizers struct {
	// AnthropicAPIKey enables Claude counts through Anthropic's token
	// counting endpoint; "" uses $ANTHROPIC_API_KEY, if set.
	AnthropicAPIKey string `yaml:"anthropic_api_key"`
	// AnthropicModel is the model whose tokenizer counts.
	AnthropicModel string `yaml:"anthropic_model"`
	// O200k counts with the GPT-4o family's encoding; its vocabulary (3.6 MB)
	// is downloaded to the user cache dir on first use.
	O200k bool `yaml:"o200k"`
}

type Pipeline struct {
	Receivers []string `yaml:"receivers"`
	Filters   []string `yaml:"filters,omitempty"`
	Exporters []string `yaml:"exporters,omitempty"`
	// Router overrides the top-level router for this pipeline.
	Router *router.Config `yaml:"router,omitempty"`
}

// RouterFor returns the router config a pipeline uses.
func (c *Config) RouterFor(p Pipeline) *router.Config {
	if p.Router != nil {
		return p.Router
	}
	return &c.Router
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func Parse(data []byte) (*Config, error) {
	var c Config
	if err := Decode(data, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &c, nil
}

// Decode unmarshals strictly: unknown fields are errors.
func Decode(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}

// DecodeNode strictly decodes a component's raw node over out (which holds defaults).
func DecodeNode(n Node, out any) error {
	if n.Kind == 0 {
		return nil
	}
	data, err := yaml.Marshal(&n)
	if err != nil {
		return err
	}
	return Decode(data, out)
}

func (c *Config) Validate() error {
	if len(c.Pipelines) == 0 {
		return errors.New("no pipelines")
	}
	var errs []error
	for kind, comps := range c.sections() {
		for id := range comps {
			if _, err := component.ParseID(id); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", kind, err))
			}
		}
	}
	for _, name := range slices.Sorted(maps.Keys(c.Pipelines)) {
		p := c.Pipelines[name]
		if len(p.Receivers) == 0 {
			errs = append(errs, fmt.Errorf("pipeline %s: no receivers", name))
		}
		errs = append(errs, refs(name, component.KindReceiver, p.Receivers, c.Receivers)...)
		errs = append(errs, refs(name, component.KindFilter, p.Filters, c.Filters)...)
		errs = append(errs, refs(name, component.KindExporter, p.Exporters, c.Exporters)...)
		errs = append(errs, refs(name, component.KindOptimizer, c.RouterFor(p).Optimizers(), c.Optimizers)...)
	}
	return errors.Join(errs...)
}

func (c *Config) sections() map[component.Kind]Components {
	return map[component.Kind]Components{
		component.KindExtension: c.Extensions,
		component.KindReceiver:  c.Receivers,
		component.KindFilter:    c.Filters,
		component.KindOptimizer: c.Optimizers,
		component.KindExporter:  c.Exporters,
	}
}

func refs(pipeline string, kind component.Kind, ids []string, defined Components) []error {
	var errs []error
	for _, id := range ids {
		if _, ok := defined[id]; !ok {
			errs = append(errs, fmt.Errorf("pipeline %s: %s %q not defined", pipeline, kind, id))
		}
	}
	return errs
}
