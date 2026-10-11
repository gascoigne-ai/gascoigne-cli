package stdinreceiver

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
	next := &capture{}
	r := &Receiver{cfg: &Config{Kind: payload.JSON}, next: next, in: strings.NewReader("{}\n")}
	if err := r.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if next.got.Kind() != payload.JSON || next.got.Text() != "{}\n" {
		t.Fatalf("got %+v", next.got)
	}
}
