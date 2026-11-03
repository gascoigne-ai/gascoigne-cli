package externaloptimizer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

// baseEnv is all a child inherits; the rest comes from Config.PassEnv and Config.Env.
// Scrubbing keeps secrets (API keys, tokens) away from third-party code.
var baseEnv = []string{"PATH", "LANG", "LC_ALL", "TMPDIR", "TERM"}

const stderrKeep = 2048

// command builds the child process: argv without a shell, scrubbed environment,
// its own process group so a timeout kills grandchildren too.
func (c *Config) command(ctx context.Context, extraEnv ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Command[0], c.Command[1:]...)
	cmd.Dir = c.Dir
	cmd.Env = c.environ(extraEnv)
	cmd.WaitDelay = time.Second
	isolate(cmd)
	return cmd
}

func (c *Config) environ(extra []string) []string {
	env := []string{"GASCOIGNE_PROTOCOL=" + fmt.Sprint(Version)}
	for _, k := range slices.Concat(baseEnv, c.PassEnv) {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	for k, v := range c.Env {
		env = append(env, k+"="+v)
	}
	return append(env, extra...)
}

// limitBuffer fails the write that would grow it past max, which stops a
// runaway child through the broken pipe instead of filling memory.
type limitBuffer struct {
	bytes.Buffer
	max int
}

var errTooBig = errors.New("output over max_output_bytes")

func (b *limitBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.max {
		return 0, errTooBig
	}
	return b.Buffer.Write(p)
}

// tailBuffer keeps the last stderrKeep bytes: the end of a trace is the useful part.
type tailBuffer struct{ b []byte }

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if len(t.b) > stderrKeep {
		t.b = t.b[len(t.b)-stderrKeep:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string { return strings.TrimSpace(string(t.b)) }

// runOnce runs the child to completion with stdin and returns its stdout.
func (c *Config) runOnce(ctx context.Context, stdin []byte, extraEnv []string) (out []byte, stderr string, err error) {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	var so = limitBuffer{max: c.MaxOutputBytes}
	var se tailBuffer
	cmd := c.command(ctx, extraEnv...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = &so, &se
	err = cmd.Run()
	switch {
	case ctx.Err() != nil:
		err = fmt.Errorf("timed out after %s", c.Timeout)
	case errors.Is(err, errTooBig):
		err = errTooBig
	case err != nil:
		err = fmt.Errorf("%w: %s", err, se)
	}
	return so.Bytes(), se.String(), err
}
