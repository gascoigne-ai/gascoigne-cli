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

// A pipeline lists the optimizers it runs; the shared router stays as it is
// for the others, and routes keep their order inside the allowed set.
func TestPipelineOptimizers(t *testing.T) {
	y := valid + `  hook:
    receivers: [stdin]
    optimizers: [passthrough]
`
	c, err := Parse([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		pipeline string
		def      []string
		route    []string
	}{
		{"cli", []string{"passthrough/big"}, []string{"passthrough"}},
		{"hook", []string{"passthrough/big"}, []string{"passthrough"}}, // "passthrough" also lists its instance passthrough/big
	}
	for _, tt := range tests {
		t.Run(tt.pipeline, func(t *testing.T) {
			r := c.RouterFor(c.Pipelines[tt.pipeline])
			if strings.Join(r.Default, ",") != strings.Join(tt.def, ",") {
				t.Errorf("default = %v, want %v", r.Default, tt.def)
			}
			if got := r.Routes[0].Optimizers; strings.Join(got, ",") != strings.Join(tt.route, ",") {
				t.Errorf("route = %v, want %v", got, tt.route)
			}
		})
	}
}

func TestPipelineOptimizersErrors(t *testing.T) {
	y := valid + `  hook:
    receivers: [stdin]
    optimizers: [nope]
`
	if _, err := Parse([]byte(y)); err == nil || !strings.Contains(err.Error(), `pipeline hook: optimizer "nope" not defined`) {
		t.Fatalf("err = %v, want undefined optimizer error", err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name, prefix string
		want         int
		err          string
	}{
		{"missing means 1", "", 1, ""},
		{"1", "version: 1\n", 1, ""},
		{"2 is too new", "version: 2\n", 0, "version 2 is newer than this gascoigne supports (1)"},
		{"0", "version: 0\n", 0, "version"},
		{"text", "version: x\n", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := Parse([]byte(tt.prefix + valid))
			if tt.err != "" || tt.name == "text" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want containing %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.Version != tt.want {
				t.Errorf("Version = %d, want %d", c.Version, tt.want)
			}
		})
	}
}

func TestCheckVersion(t *testing.T) {
	tests := []struct {
		name, data string
		want       int
		err        string
	}{
		{"empty", "", 1, ""},
		{"comments only", "# nothing\n", 1, ""},
		{"not a mapping", "- a\n- b\n", 1, ""},
		{"missing", "router: {}\n", 1, ""},
		{"null", "version:\n", 1, ""},
		{"1", "version: 1\n", 1, ""},
		{"2", "version: 2\n", 0, "version 2 is newer than this gascoigne supports (1); update gascoigne"},
		{"0", "version: 0\n", 0, "version must be a whole number of 1 or more"},
		{"negative", "version: -3\n", 0, "version must be a whole number of 1 or more"},
		{"text", "version: x\n", 0, "version must be a whole number of 1 or more"},
		{"fraction", "version: 1.5\n", 0, "version must be a whole number of 1 or more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckVersion([]byte(tt.data))
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want containing %q", err, tt.err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("CheckVersion = %d, %v, want %d", got, err, tt.want)
			}
		})
	}
}
