// Package execreceiver runs a command and feeds its combined output through
// the pipeline, so agents can wrap any shell command: gascoigne exec -- go test ./...
// The command comes from the context (see With), set per CLI invocation.
package execreceiver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress/watch"
	"github.com/lukaszb/gascoigne/payload"
)

// Invocation is one command run: Args in, ExitCode filled in by Run.
// Watch: the command rebuilds until stopped (tsc -w); each finished cycle
// goes through the pipeline as soon as its result line appears.
type Invocation struct {
	Args     []string
	Stdin    io.Reader
	Watch    bool
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
	var err error
	if inv.Watch {
		err = r.watch(ctx, cmd, &out)
	} else {
		err = cmd.Run()
	}
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
	if inv.Watch && strings.TrimSpace(out.String()) == "" {
		return nil
	}
	return r.process(ctx, inv, out.String())
}

func (r *Receiver) process(ctx context.Context, inv *Invocation, text string) error {
	p := payload.New(r.cfg.Kind, text)
	p.Set(payload.AttrCmd, strings.Join(inv.Args, " "))
	_, err := r.next.Process(ctx, p)
	return err
}

// watch runs cmd and processes its output a cycle at a time; what follows
// the last cycle is left in rest.
func (r *Receiver) watch(ctx context.Context, cmd *exec.Cmd, rest *bytes.Buffer) error {
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		pw.Close()
		done <- err
	}()
	inv, _ := ctx.Value(ctxKey{}).(*Invocation)
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var cycle strings.Builder
	for sc.Scan() {
		cycle.WriteString(sc.Text() + "\n")
		if !watch.End(sc.Text()) {
			continue
		}
		if err := r.process(ctx, inv, cycle.String()); err != nil {
			return err
		}
		cycle.Reset()
	}
	io.Copy(io.Discard, pr) // a line over the buffer: drain so the command can exit
	rest.WriteString(cycle.String())
	return <-done
}
