package externaloptimizer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// rpc keeps one child alive across calls: newline-delimited json both ways,
// one request at a time. The child starts on first use, so a broken command
// costs one failed optimization and not the whole pipeline start.
type rpc struct {
	cfg *Config
	log *slog.Logger

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr *tailBuffer
	cancel context.CancelFunc
	nextID int
}

func newRPCCodec(cfg *Config, log *slog.Logger) codec { return &rpc{cfg: cfg, log: log} }

func (r *rpc) call(ctx context.Context, req Request) (Response, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resp, err := r.roundTrip(ctx, req)
	if err == nil || ctx.Err() != nil {
		return resp, err
	}
	// A dead child (crash, EOF) gets one restart; a bad answer does not.
	if !errors.Is(err, errChildGone) {
		return resp, err
	}
	r.log.Warn("external child gone, restarting", "err", err)
	return r.roundTrip(ctx, req)
}

var errChildGone = errors.New("child gone")

func (r *rpc) roundTrip(ctx context.Context, req Request) (Response, error) {
	if err := r.ensure(ctx); err != nil {
		return Response{}, err
	}
	r.nextID++
	req.Version, req.ID = Version, r.nextID
	line, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	resp, err := r.exchange(ctx, append(line, '\n'))
	if err != nil {
		r.stop()
		return Response{}, err
	}
	if resp.ID != req.ID {
		r.stop() // out of sync: the next call starts clean
		return Response{}, fmt.Errorf("child answered id %d, want %d", resp.ID, req.ID)
	}
	return resp, nil
}

// exchange writes one line and reads one line within the timeout; on timeout
// the child is killed because its reader cannot be interrupted.
func (r *rpc) exchange(ctx context.Context, line []byte) (Response, error) {
	type result struct {
		resp Response
		err  error
	}
	done := make(chan result, 1)
	// The goroutine may outlive a timeout; it keeps its own handles so a
	// restart cannot race with it.
	stdin, stdout, stderr := r.stdin, r.stdout, r.stderr
	go func() {
		if _, err := stdin.Write(line); err != nil {
			done <- result{err: fmt.Errorf("%w: write: %v", errChildGone, err)}
			return
		}
		resp, err := r.readResponse(stdout, stderr)
		done <- result{resp, err}
	}()
	timer := time.NewTimer(r.cfg.Timeout)
	defer timer.Stop()
	select {
	case res := <-done:
		return res.resp, res.err
	case <-timer.C:
		r.stop()
		return Response{}, fmt.Errorf("timed out after %s", r.cfg.Timeout)
	case <-ctx.Done():
		r.stop()
		return Response{}, ctx.Err()
	}
}

func (r *rpc) readResponse(stdout *bufio.Reader, stderr *tailBuffer) (Response, error) {
	var buf bytes.Buffer
	for {
		part, isPrefix, err := stdout.ReadLine()
		if err != nil {
			return Response{}, fmt.Errorf("%w: %v: %s", errChildGone, err, stderr)
		}
		if buf.Len()+len(part) > r.cfg.MaxOutputBytes {
			return Response{}, errTooBig
		}
		buf.Write(part)
		if !isPrefix {
			break
		}
	}
	var resp Response
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		return Response{}, fmt.Errorf("bad json response: %w", err)
	}
	return resp, nil
}

// ensure starts the child and does the hello handshake when none is running.
func (r *rpc) ensure(ctx context.Context) error {
	if r.cmd != nil {
		return nil
	}
	// The child outlives one call, so it must not die with the call's context.
	cctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := r.cfg.command(cctx)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return err
	}
	r.stderr = &tailBuffer{}
	cmd.Stderr = r.stderr
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("start: %w", err)
	}
	r.cmd, r.stdin, r.stdout, r.cancel = cmd, stdin, bufio.NewReader(stdout), cancel
	if _, err := r.exchange(ctx, []byte("{\"hello\":1}\n")); err != nil {
		r.stop()
		return fmt.Errorf("handshake: %w", err)
	}
	return nil
}

// stop kills the child and forgets it; the next call starts a new one.
func (r *rpc) stop() {
	if r.cmd == nil {
		return
	}
	r.stdin.Close()
	r.cancel()
	r.cmd.Wait() //nolint:errcheck // killed on purpose
	r.cmd = nil
}

func (r *rpc) close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stop()
}
