package config

import (
	"slices"
	"strings"
	"testing"
)

func TestSet(t *testing.T) {
	c, err := ParseWith([]byte(valid), Overrides{Set: []string{
		"filters.minsize.min_tokens=50",
		"router.default=[passthrough]",
		"optimizers.passthrough/extra={}",
		"pipelines.cli.filters=[]",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var mc struct {
		MinTokens int `yaml:"min_tokens"`
	}
	if err := DecodeNode(c.Filters["minsize"], &mc); err != nil || mc.MinTokens != 50 {
		t.Fatalf("min_tokens = %d, %v", mc.MinTokens, err)
	}
	if !slices.Equal(c.Router.Default, []string{"passthrough"}) {
		t.Fatalf("default = %v", c.Router.Default)
	}
	if _, ok := c.Optimizers["passthrough/extra"]; !ok || len(c.Pipelines["cli"].Filters) != 0 {
		t.Fatalf("optimizers %v, cli filters %v", c.Optimizers, c.Pipelines["cli"].Filters)
	}
}

func TestSetErrors(t *testing.T) {
	for _, set := range []string{"nopath", "=1", "router.default.x=1", "bogus=1", "router.default=[nope]"} {
		if _, err := ParseWith([]byte(valid), Overrides{Set: []string{set}}); err == nil {
			t.Errorf("set %q: want error", set)
		}
	}
}

func TestOnlyDisable(t *testing.T) {
	c, err := ParseWith([]byte(valid), Overrides{Disable: []string{"passthrough/big"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Router.Default) != 0 || !slices.Equal(c.Router.Routes[0].Optimizers, []string{"passthrough"}) {
		t.Fatalf("disable: %+v", c.Router)
	}
	c, err = ParseWith([]byte(valid), Overrides{Only: []string{"passthrough/big"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Router.Routes[0].Optimizers) != 0 || !slices.Equal(c.Router.Default, []string{"passthrough/big"}) {
		t.Fatalf("only: %+v", c.Router)
	}
	if _, err := ParseWith([]byte(valid), Overrides{Only: []string{"nope"}}); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("unknown optimizer: %v", err)
	}
}
