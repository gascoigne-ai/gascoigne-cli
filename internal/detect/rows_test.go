package detect

import "testing"

func TestRows(t *testing.T) {
	for _, r := range rows {
		t.Run(r.Type+"/"+r.Cmd+r.Sniff, func(t *testing.T) {
			if len(r.Cmds) > 0 {
				if r.Cmd == "" {
					t.Fatal("no command example")
				}
				if got := FromCmd(r.Cmd); got != r.Type {
					t.Errorf("command %q detected as %q", r.Cmd, got)
				}
			}
			if r.Sniff != "" {
				if r.Sample == "" {
					t.Fatal("no content example")
				}
				if got := Sniff(r.Sample); got != r.Type {
					t.Errorf("content sniffed as %q", got)
				}
			}
		})
	}
}
