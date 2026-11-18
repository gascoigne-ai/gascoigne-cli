package configs

import (
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestDefaultMixedRouteFirst keeps the mixed-output route first, so a blob
// of several families skips the family engines.
func TestDefaultMixedRouteFirst(t *testing.T) {
	var cfg struct {
		Router struct {
			Routes []struct {
				Name  string `yaml:"name"`
				Match struct {
					Attrs map[string]string `yaml:"attrs"`
				} `yaml:"match"`
				Optimizers []string `yaml:"optimizers"`
			} `yaml:"routes"`
		} `yaml:"router"`
	}
	if err := yaml.Unmarshal(Default, &cfg); err != nil {
		t.Fatal(err)
	}
	routes := cfg.Router.Routes
	if len(routes) == 0 || routes[0].Name != "mixed" {
		t.Fatalf("first route %+v, want mixed", routes)
	}
	if routes[0].Match.Attrs["mixed"] != "true" {
		t.Errorf("match attrs %v", routes[0].Match.Attrs)
	}
	if want := []string{"dense", "paths", "truncate"}; !slices.Equal(routes[0].Optimizers, want) {
		t.Errorf("chain %v, want %v", routes[0].Optimizers, want)
	}
}
