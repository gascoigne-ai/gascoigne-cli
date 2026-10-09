package detectfilter

import (
	"log/slog"
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
	f := &Filter{log: slog.New(slog.DiscardHandler)}
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
	f := &Filter{log: slog.New(slog.DiscardHandler)}
	f.Filter(t.Context(), p)
	if p.Blocks[0].Type != detect.GitStatus {
		t.Fatalf("type %q", p.Blocks[0].Type)
	}
}
