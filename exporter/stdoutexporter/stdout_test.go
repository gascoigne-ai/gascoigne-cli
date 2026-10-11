package stdoutexporter

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func TestExport(t *testing.T) {
	optimized := payload.New(payload.CmdOutput, "short")
	optimized.Set(payload.AttrRawID, "abc")
	optimized.Set(payload.AttrRawTokens, "100")
	optimized.Set(payload.AttrOutTokens, "2")
	top := optimized.Clone()
	top.Set(AttrHint, HintTop)

	tests := []struct {
		name string
		hint bool
		p    *payload.Payload
		want string
	}{
		{"verbatim", true, payload.New(payload.CmdOutput, "raw"), "raw"},
		{"hint", true, optimized, "short\n[gascoigne: optimized 100→2 tokens · full: gascoigne raw abc]\n"},
		{"hint off", false, optimized, "short"},
		{"hint on top", true, top, "[gascoigne: optimized 100→2 tokens · full: gascoigne raw abc]\nshort"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			e := &Exporter{cfg: &Config{Hint: tt.hint}, w: &b}
			if err := e.Export(t.Context(), tt.p); err != nil {
				t.Fatal(err)
			}
			if b.String() != tt.want {
				t.Fatalf("got %q, want %q", b.String(), tt.want)
			}
		})
	}
}
