package payload

import "testing"

func TestKey(t *testing.T) {
	tests := []struct {
		attrs map[string]string
		want  string
	}{
		{nil, ""},
		{map[string]string{AttrCmd: "  git   status "}, "cmd:git status"},
		{map[string]string{AttrCmd: "cat x", AttrFile: "/a/x"}, "file:/a/x"},
		{map[string]string{AttrTool: "Read"}, "tool:Read"},
	}
	for _, tt := range tests {
		p := &Payload{Attrs: tt.attrs}
		if got := p.Key(); got != tt.want {
			t.Errorf("Key(%v) = %q, want %q", tt.attrs, got, tt.want)
		}
	}
}

func TestCloneIsDeep(t *testing.T) {
	p := New(CmdOutput, "a")
	p.Set("k", "v")
	c := p.Clone()
	c.Blocks[0].Text = "b"
	c.Set("k", "w")
	if p.Text() != "a" || p.Attrs["k"] != "v" {
		t.Fatalf("clone mutated original: %+v", p)
	}
	if p.SameContent(c) {
		t.Fatal("SameContent = true after change")
	}
}

func TestText(t *testing.T) {
	p := &Payload{Blocks: []Block{{Message, "a"}, {Message, "b"}}}
	if got := p.Text(); got != "a\nb" {
		t.Fatalf("Text = %q", got)
	}
}
