// Package hookreceiver optimizes output an agent already captured, handed over
// by an agent hook after a tool ran (gascoigne hook codex). The text comes from
// an option set in the factory (see WithOutput), once per hook call; the result goes back on the Output.
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

// Option sets one input of the hook receiver, in the factory.
type Option func(*Receiver)

// WithOutput sets the tool result the receiver optimizes; Run fills in its Result.
func WithOutput(out *Output) Option {
	return func(rec *Receiver) { rec.out = out }
}

// Receiver sends the output set by WithOutput down the pipeline.
type Receiver struct {
	component.Nop
	cfg  *Config
	next component.Pipeline
	out  *Output
}

// Run processes the output set by WithOutput; without one it does nothing.
func (r *Receiver) Run(ctx context.Context) error {
	out := r.out
	if out == nil {
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
