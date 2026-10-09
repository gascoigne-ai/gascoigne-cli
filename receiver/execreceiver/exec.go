// Package execreceiver runs a command and feeds its combined output through
// the pipeline, so agents can wrap any shell command: gascoigne exec -- go test ./...
// The command comes from the context (see With), set per CLI invocation.
package execreceiver

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

// Invocation is one command run: Args in, ExitCode filled in by Run.
type Invocation struct {
	Args     []string
	Stdin    io.Reader
	ExitCode int
}

type ctxKey struct{}

// With attaches inv to ctx for the exec receiver.
func With(ctx context.Context, inv *Invocation) context.Context {
	return context.WithValue(ctx, ctxKey{}, inv)
}

type Receiver struct {
	component.Nop
	cfg  *Config
	next component.Pipeline
}

// Run executes the invocation from ctx; without one it does nothing.
func (r *Receiver) Run(ctx context.Context) error {
	inv, ok := ctx.Value(ctxKey{}).(*Invocation)
	if !ok || len(inv.Args) == 0 {
		return nil
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, inv.Args[0], inv.Args[1:]...)
	cmd.Stdout, cmd.Stderr = &out, &out // one writer keeps stdout/stderr order
	cmd.Stdin = inv.Stdin
	if cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		inv.ExitCode = exitErr.ExitCode()
	case errors.Is(err, exec.ErrNotFound):
		inv.ExitCode = 127
		return err
	case err != nil:
		inv.ExitCode = 1
		return err
	}
	p := payload.New(r.cfg.Kind, out.String())
	p.Set(payload.AttrCmd, strings.Join(inv.Args, " "))
	_, err = r.next.Process(ctx, p)
	return err
}
