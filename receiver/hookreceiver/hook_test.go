package hookreceiver

import (
	"context"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type tag struct{}

func (tag) Process(_ context.Context, p *payload.Payload) (*payload.Payload, error) {
	p.Set(payload.AttrRawID, "r1")
	return p, nil
}

func TestRun(t *testing.T) {
	r := &Receiver{cfg: &Config{Kind: payload.CmdOutput}, next: tag{}}
	out := &Output{Text: "ok\n", Cmd: "go test ./..."}
	if err := r.Run(With(t.Context(), out)); err != nil {
		t.Fatal(err)
	}
	if out.Result == nil || out.Result.Text() != "ok\n" || out.Result.Attrs[payload.AttrCmd] != "go test ./..." || out.Result.Attrs[payload.AttrRawID] != "r1" {
		t.Fatalf("got %+v", out.Result)
	}
}

func TestRunWithoutOutput(t *testing.T) {
	r := &Receiver{cfg: &Config{Kind: payload.CmdOutput}, next: tag{}}
	if err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
}
