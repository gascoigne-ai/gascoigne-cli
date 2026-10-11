package detectfilter

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func TestFilterTagsEachBlock(t *testing.T) {
	p := &payload.Payload{Blocks: []payload.Block{
		{Kind: payload.ToolResult, Text: `{"a":1}`},
		{Kind: payload.ToolResult, Text: "diff --git a/x b/x\n@@ -1 +1 @@\n"},
		{Kind: payload.ToolResult, Text: "kept", Type: detect.Log},
	}}
	f := &Filter{cfg: NewFactory().DefaultConfig().(*Config), log: slog.New(slog.DiscardHandler)}
	if d, err := f.Filter(t.Context(), p); d != component.Continue || err != nil {
		t.Fatalf("decision %v, %v", d, err)
	}
	want := []string{detect.JSON, detect.Diff, detect.Log}
	for i, b := range p.Blocks {
		if b.Type != want[i] {
			t.Errorf("block %d type %q, want %q", i, b.Type, want[i])
		}
	}
}

func TestFilterUsesCmdHint(t *testing.T) {
	p := payload.New(payload.CmdOutput, "anything")
	p.Set(payload.AttrCmd, "git status")
	f := &Filter{cfg: NewFactory().DefaultConfig().(*Config), log: slog.New(slog.DiscardHandler)}
	f.Filter(t.Context(), p)
	if p.Blocks[0].Type != detect.GitStatus {
		t.Fatalf("type %q", p.Blocks[0].Type)
	}
}

func TestEditSafe(t *testing.T) {
	small := "# service config\nport: 8080\nname: shop\n"
	big := strings.Repeat("- name: pkg\n  version: 1.2.3\n", 2000)
	tests := []struct{ cmd, text, want string }{
		{"cat config.yaml", small, detect.Code},
		{"sed -n 1,40p deploy/values.yaml", small, detect.Code},
		{"cat pnpm-lock.yaml", big, detect.YAML},
		{"kubectl get cm x -o yaml", small, detect.YAML}, // command output, not a file
	}
	f := &Filter{cfg: NewFactory().DefaultConfig().(*Config), log: slog.New(slog.DiscardHandler)}
	for _, tt := range tests {
		p := payload.New(payload.CmdOutput, tt.text)
		p.Set(payload.AttrCmd, tt.cmd)
		f.Filter(t.Context(), p)
		if got := p.Blocks[0].Type; got != tt.want && !(tt.cmd == "kubectl get cm x -o yaml" && detect.Family(got) == "k8s") {
			t.Errorf("%s: %s, want %s", tt.cmd, got, tt.want)
		}
	}
}
