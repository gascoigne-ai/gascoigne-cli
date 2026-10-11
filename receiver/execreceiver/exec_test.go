package execreceiver

import (
	"context"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
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

type collect struct{ got []string }

func (c *collect) Process(_ context.Context, p *payload.Payload) (*payload.Payload, error) {
	c.got = append(c.got, p.Text())
	return p, nil
}

func TestWatchProcessesEachCycle(t *testing.T) {
	script := `echo "[7:00:01 AM] Starting compilation in watch mode..."
echo "a.ts(1,1): error TS2304: Cannot find name 'x'." >&2
echo "[7:00:02 AM] Found 1 error. Watching for file changes."
echo "[7:00:05 AM] File change detected. Starting incremental compilation..."
echo "[7:00:06 AM] Found 0 errors. Watching for file changes."
echo "Terminating"
exit 130`
	next := &collect{}
	r := &Receiver{cfg: &Config{Kind: payload.CmdOutput}, next: next}
	inv := &Invocation{Args: []string{"sh", "-c", script}, Stdin: strings.NewReader(""), Watch: true}
	if err := r.Run(With(t.Context(), inv)); err != nil {
		t.Fatal(err)
	}
	if len(next.got) != 3 || !strings.HasSuffix(next.got[0], "Found 1 error. Watching for file changes.\n") ||
		!strings.Contains(next.got[0], "TS2304") || next.got[2] != "Terminating\n" || inv.ExitCode != 130 {
		t.Fatalf("cycles %q, exit %d", next.got, inv.ExitCode)
	}
}
