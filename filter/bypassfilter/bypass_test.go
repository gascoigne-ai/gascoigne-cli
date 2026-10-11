package bypassfilter

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		name, attr, env string
		want            component.Decision
	}{
		{"none", "", "", component.Continue},
		{"attr", "true", "", component.Bypass},
		{"attr false", "0", "", component.Continue},
		{"env", "", "1", component.Bypass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(DefaultEnv, tt.env)
			p := payload.New(payload.CmdOutput, "x")
			if tt.attr != "" {
				p.Set(payload.AttrRaw, tt.attr)
			}
			f := &Filter{cfg: &Config{Env: DefaultEnv}}
			if got, _ := f.Filter(t.Context(), p); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
