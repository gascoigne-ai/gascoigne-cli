// Package execreceiver runs a command and feeds its combined output through
// the pipeline, so agents can wrap any shell command: gascoigne exec -- go test ./...
// The command comes from an option set in the factory (see WithInvocation), once per CLI invocation.
package execreceiver

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/watch"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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

// Option sets one input of the exec receiver, in the factory.
type Option func(*Receiver)

// WithInvocation sets the command run by the receiver; Run fills in its ExitCode.
func WithInvocation(inv *Invocation) Option {
	return func(rec *Receiver) { rec.inv = inv }
}

// finished is how a command ended; watch cycles are processed while it still
// runs, so they have none.
type finished struct {
	exit int
	took time.Duration
}

// Receiver runs its invocation and sends the output down the pipeline.
type Receiver struct {
	component.Nop
	cfg  *Config
	next component.Pipeline
	inv  *Invocation
}

// Run executes the invocation set by WithInvocation; without one it does nothing.
func (r *Receiver) Run(ctx context.Context) error {
	inv := r.inv
	if inv == nil || len(inv.Args) == 0 {
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
	start := time.Now()
	if inv.Watch {
		err = r.watch(ctx, inv, cmd, &out)
	} else {
		err = cmd.Run()
	}
	took := time.Since(start)
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
	var done *finished
	if !inv.Watch {
		done = &finished{exit: inv.ExitCode, took: took}
	}
	return r.process(ctx, inv, out.String(), done)
}

// process sends text down the pipeline with what is known of the command: the
// directory always, how it ended when done is set.
func (r *Receiver) process(ctx context.Context, inv *Invocation, text string, done *finished) error {
	p := payload.New(r.cfg.Kind, text)
	p.Set(payload.AttrCmd, strings.Join(inv.Args, " "))
	if cwd, err := os.Getwd(); err == nil {
		p.Set(payload.AttrCwd, cwd)
	}
	if done != nil {
		p.Set(payload.AttrExit, strconv.Itoa(done.exit))
		p.Set(payload.AttrRunMS, strconv.FormatInt(done.took.Milliseconds(), 10))
	}
	_, err := r.next.Process(ctx, p)
	return err
}

// watch runs cmd and processes its output a cycle at a time; what follows
// the last cycle is left in rest.
func (r *Receiver) watch(ctx context.Context, inv *Invocation, cmd *exec.Cmd, rest *bytes.Buffer) error {
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
	// Closing the read side fails the command's writes, so it cannot block on a
	// full pipe when we return early; then wait for it.
	defer pr.Close()
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var cycle strings.Builder
	for sc.Scan() {
		cycle.WriteString(sc.Text())
		cycle.WriteString("\n")
		if !watch.End(sc.Text()) {
			continue
		}
		if err := r.process(ctx, inv, cycle.String(), nil); err != nil {
			pr.Close()
			<-done
			return err
		}
		cycle.Reset()
	}
	if err := sc.Err(); err != nil {
		slog.Debug("exec watch: scan output", "err", err)
	}
	io.Copy(io.Discard, pr) // a line over the buffer: drain so the command can exit
	rest.WriteString(cycle.String())
	return <-done
}
