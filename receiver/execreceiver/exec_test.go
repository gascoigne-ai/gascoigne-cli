package execreceiver

import (
	"context"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/payload"
)

type capture struct{ got *payload.Payload }

func (c *capture) Process(_ context.Context, p *payload.Payload) (*payload.Payload, error) {
	c.got = p
	return p, nil
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantCode int
		wantErr  bool
	}{
		{"ok", []string{"sh", "-c", "echo out; echo err >&2"}, "out\nerr\n", 0, false},
		{"exit code", []string{"sh", "-c", "echo hi; exit 3"}, "hi\n", 3, false},
		{"missing binary", []string{"definitely-not-a-binary-xyz"}, "", 127, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := &capture{}
			r := &Receiver{cfg: &Config{Kind: payload.CmdOutput}, next: next}
			inv := &Invocation{Args: tt.args, Stdin: strings.NewReader("")}
			err := r.Run(With(t.Context(), inv))
			if (err != nil) != tt.wantErr || inv.ExitCode != tt.wantCode {
				t.Fatalf("err %v, code %d", err, inv.ExitCode)
			}
			if tt.wantErr {
				return
			}
			if next.got.Text() != tt.wantOut || next.got.Attrs[payload.AttrCmd] != strings.Join(tt.args, " ") {
				t.Fatalf("payload %+v", next.got)
			}
		})
	}
}

func TestRunWithoutInvocationIsNoop(t *testing.T) {
	next := &capture{}
	r := &Receiver{cfg: &Config{Kind: payload.CmdOutput}, next: next}
	if err := r.Run(t.Context()); err != nil || next.got != nil {
		t.Fatalf("err %v, got %v", err, next.got)
	}
}
