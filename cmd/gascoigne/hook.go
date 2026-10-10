package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"strings"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/receiver/hookreceiver"
)

// hookInput is the event agent hooks send on stdin. Claude Code, Codex and
// Cursor share these fields; the pi extension sends the same shape.
type hookInput struct {
	SessionID    string          `json:"session_id"`
	ToolInput    map[string]any  `json:"tool_input"`
	ToolResponse json.RawMessage `json:"tool_response"`
}

// hookFunc answers one hook event; nil means "leave the tool call alone".
// gascoigne is how the agent's shell reaches this binary.
type hookFunc func(ctx context.Context, c *common, in hookInput, gascoigne string) (any, error)

// cmdHook handles an agent hook event. It never fails the agent's tool call:
// on any problem it prints nothing and the call goes on unchanged.
func cmdHook(ctx context.Context, args []string) error {
	var c common
	fs := flag.NewFlagSet("hook", flag.ContinueOnError)
	c.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	a, ok := agents[name]
	if !ok {
		return fmt.Errorf("hook: want an agent: %s", agentNames())
	}
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne hook: read event:", err)
		return nil
	}
	if c.agent == "" && os.Getenv("GASCOIGNE_AGENT") == "" {
		c.agent = name
	}
	if c.sessionID() == "" {
		c.session = in.SessionID
	}
	reply, err := a.hook(ctx, &c, in, self())
	if err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne hook:", err)
		return nil
	}
	if reply == nil {
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(reply)
}

// rewriteHook routes shell tool calls through gascoigne exec before they run;
// reply shapes the agent's answer (updated is nil when nothing changed).
func rewriteHook(reply func(updated map[string]any) any) hookFunc {
	return func(_ context.Context, _ *common, in hookInput, gascoigne string) (any, error) {
		cmd, _ := in.ToolInput["command"].(string)
		background, _ := in.ToolInput["run_in_background"].(bool)
		wrapped, ok := wrapCommand(cmd, gascoigne)
		if !ok || background {
			return reply(nil), nil
		}
		updated := maps.Clone(in.ToolInput)
		updated["command"] = wrapped
		return reply(updated), nil
	}
}

// optimizeHook optimizes shell output after the tool ran, for agents whose
// hooks can replace a result but not rewrite a command without approving it.
func optimizeHook(reply func(text string) any) hookFunc {
	return func(ctx context.Context, c *common, in hookInput, _ string) (any, error) {
		cmd, _ := in.ToolInput["command"].(string)
		text, code, ok := shellOutput(in.ToolResponse)
		if !ok || strings.Contains(strings.ToLower(cmd), "gascoigne") {
			return nil, nil
		}
		p, err := c.optimize(ctx, text, cmd)
		if err != nil || p == nil || p.Attrs[payload.AttrRawID] == "" {
			return nil, err
		}
		out := hinted(p)
		if code != 0 {
			out = fmt.Sprintf("exit code %d\n%s", code, out)
		}
		return reply(out), nil
	}
}

// shellOutput reads a shell tool response: plain text, or output plus exit code.
func shellOutput(raw json.RawMessage) (string, int, bool) {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, 0, true
	}
	var res struct {
		Output   *string `json:"output"`
		ExitCode int     `json:"exit_code"`
	}
	if json.Unmarshal(raw, &res) != nil || res.Output == nil {
		return "", 0, false
	}
	return *res.Output, res.ExitCode, true
}

// optimize runs text through the hook pipeline.
func (c *common) optimize(ctx context.Context, text, cmd string) (*payload.Payload, error) {
	svc, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	out := &hookreceiver.Output{Text: text, Cmd: cmd}
	runErr := svc.Run(hookreceiver.With(ctx, out), "hook")
	return out.Result, errors.Join(runErr, svc.Shutdown(context.WithoutCancel(ctx)))
}

// hinted is the optimized text plus how to get the original, as exec prints it.
func hinted(p *payload.Payload) string {
	text := p.Text()
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return fmt.Sprintf("%s[gascoigne: optimized %s→%s tokens · full: gascoigne raw %s]\n",
		text, p.Attrs[payload.AttrRawTokens], p.Attrs[payload.AttrOutTokens], p.Attrs[payload.AttrRawID])
}

// wrapCommand routes the last command of a shell line through gascoigne exec
// when gascoigne knows the tool: "cd api && go test ./..." becomes
// "cd api && gascoigne exec -- go test ./...". Pipes, redirects, subshells and
// lists other than && are left alone: the agent post-processes those itself.
func wrapCommand(line, gascoigne string) (string, bool) {
	line = strings.TrimSpace(line)
	body, merged := strings.CutSuffix(line, " 2>&1") // exec merges stderr anyway
	if strings.ContainsAny(strings.ReplaceAll(body, "&&", ""), "|;&<>`\n") || strings.Contains(body, "$(") ||
		strings.Contains(strings.ToLower(line), "gascoigne") {
		return line, false
	}
	head, last := "", body
	if i := strings.LastIndex(body, "&&"); i >= 0 {
		head, last = body[:i+2]+" ", strings.TrimSpace(body[i+2:])
		if strings.Count(head, `"`)%2 == 1 || strings.Count(head, "'")%2 == 1 {
			return line, false // the && is inside quotes
		}
	}
	env, cmd := splitEnv(last)
	if detect.FromCmd(cmd) == "" || streams(cmd) {
		return line, false
	}
	out := head + env + gascoigne + " exec -- " + cmd
	if merged {
		out += " 2>&1"
	}
	return out, true
}

// splitEnv splits leading VAR=value assignments off a command.
func splitEnv(s string) (env, cmd string) {
	for {
		f, rest, _ := strings.Cut(s, " ")
		if !strings.Contains(f, "=") || strings.HasPrefix(f, "-") || strings.ContainsAny(f, `"'`) {
			return env, s
		}
		env += f + " "
		s = strings.TrimLeft(rest, " ")
	}
}

// streams reports commands that run until stopped (followed logs, watchers,
// dev servers): exec shows output only when the command exits.
func streams(cmd string) bool {
	args := detect.Argv(cmd)
	if len(args) == 0 {
		return false
	}
	follow := args[0] == "tail" || args[0] == "journalctl" || strings.Contains(cmd, "logs")
	for _, a := range args[1:] {
		switch {
		case a == "--follow" || a == "--watch" || a == "watch" || a == "dev" || a == "serve" || a == "start" || a == "up":
			return true
		case a == "-f" && follow:
			return true
		}
	}
	return false
}

// self is how agent shells reach this binary: its name when PATH finds it, else its path.
func self() string {
	exe, err := os.Executable()
	if err != nil {
		return "gascoigne"
	}
	if p, err := exec.LookPath("gascoigne"); err == nil && sameFile(p, exe) {
		return "gascoigne"
	}
	return shellQuote(exe)
}

func sameFile(a, b string) bool {
	x, err1 := os.Stat(a)
	y, err2 := os.Stat(b)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}

// shellQuote quotes s for sh when it holds anything beyond plain path characters.
func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./:@+%,=") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
