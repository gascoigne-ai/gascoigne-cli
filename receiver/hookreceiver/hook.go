// Package hookreceiver optimizes output an agent already captured, handed over
// by an agent hook after a tool ran (gascoigne hook codex). The text comes from
// the context (see With), set per hook call; the result goes back on the Output.
package hookreceiver

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Output is one tool result: Text and Cmd in, Result filled in by Run.
type Output struct {
	Text   string
	Cmd    string
	Result *payload.Payload
}

type ctxKey struct{}

// With attaches out to ctx for the hook receiver.
func With(ctx context.Context, out *Output) context.Context {
	return context.WithValue(ctx, ctxKey{}, out)
}

type Receiver struct {
	component.Nop
	cfg  *Config
	next component.Pipeline
}

// Run processes the output from ctx; without one it does nothing.
func (r *Receiver) Run(ctx context.Context) error {
	out, ok := ctx.Value(ctxKey{}).(*Output)
	if !ok {
		return nil
	}
	p := payload.New(r.cfg.Kind, out.Text)
	if out.Cmd != "" {
		p.Set(payload.AttrCmd, out.Cmd)
	}
	res, err := r.next.Process(ctx, p)
	out.Result = res
	return err
}
