package config

import (
	"strings"
	"testing"
)

const valid = `
receivers: { stdin: {} }
filters: { minsize: { min_tokens: 5 } }
optimizers: { passthrough: {}, passthrough/big: {} }
exporters: { stdout: {} }
router:
  routes:
    - match: { attrs: { cmd: "git *" } }
      optimizers: [passthrough]
  default: [passthrough/big]
pipelines:
  cli: { receivers: [stdin], filters: [minsize], exporters: [stdout] }
`

func TestParseValid(t *testing.T) {
	c, err := Parse([]byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	var mc struct {
		MinTokens int `yaml:"min_tokens"`
	}
	if err := DecodeNode(c.Filters["minsize"], &mc); err != nil || mc.MinTokens != 5 {
		t.Fatalf("DecodeNode = %+v, %v", mc, err)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, yaml, want string
	}{
		{"unknown field", valid + "bogus: 1\n", "field bogus not found"},
		{"no pipelines", "receivers: { stdin: {} }\n", "no pipelines"},
		{"missing receiver", strings.Replace(valid, "receivers: [stdin]", "receivers: [nope]", 1), `receiver "nope" not defined`},
		{"missing filter", strings.Replace(valid, "filters: [minsize]", "filters: [nope]", 1), `filter "nope" not defined`},
		{"missing optimizer", strings.Replace(valid, "default: [passthrough/big]", "default: [nope]", 1), `optimizer "nope" not defined`},
		{"bad id", strings.Replace(valid, "stdout: {}", "a/b/c: {}", 1), "invalid component id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestPipelineRouterOverride(t *testing.T) {
	y := valid + `  proxy:
    receivers: [stdin]
    router: { default: [nope] }
`
	if _, err := Parse([]byte(y)); err == nil || !strings.Contains(err.Error(), "pipeline proxy") {
		t.Fatalf("err = %v, want pipeline proxy ref error", err)
	}
}
