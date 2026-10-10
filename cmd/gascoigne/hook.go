package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/exporter/stdoutexporter"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/receiver/hookreceiver"
)

// hookInput is the event agent hooks send on stdin. Claude Code, Codex and
// Cursor share these fields; the pi extension sends the same shape.
type hookInput struct {
	SessionID     string          `json:"session_id"`
	HookEventName string          `json:"hook_event_name"`
	ToolInput     map[string]any  `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
}

// hookFunc answers one hook event; nil means "leave the tool call alone".
// prefix is how the agent's shell runs a command through this binary
// ("gascoigne exec", see execPrefix).
type hookFunc func(ctx context.Context, c *common, in hookInput, prefix string) (any, error)

// cmdHook handles an agent hook event. It never fails the agent's tool call:
// on any problem it prints nothing and the call goes on unchanged.
func cmdHook(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("hook")
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
	reply, err := a.hook(ctx, &c, in, execPrefix(self(), name, in.SessionID))
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
	return func(_ context.Context, _ *common, in hookInput, prefix string) (any, error) {
		cmd, _ := in.ToolInput["command"].(string)
		background, _ := in.ToolInput["run_in_background"].(bool)
		wrapped, ok := wrapCommand(cmd, prefix)
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
		if !ok || ownCommand(cmd) {
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
	c.server = true
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

// wrapCommand routes each command of a shell line through gascoigne exec
// (except builtins, quiet and interactive commands, see passthrough): "cd api && go test ./..." becomes
// "cd api && gascoigne exec -- go test ./...", and "go test ./... | tail -5"
// becomes "gascoigne exec -- go test ./... | tail -5" (before head, exec
// prints its hint first). Commands may be joined by && or ;, and only
// head/tail may follow a pipe: other filters would see optimized text instead
// of what they search. Redirects, subshells, other lists, loops and
// conditionals are left alone.
func wrapCommand(line, prefix string) (string, bool) {
	line = strings.TrimSpace(line)
	if ownCommand(line) {
		return line, false
	}
	body, pipe := splitPipe(line)
	body, merged := strings.CutSuffix(body, " 2>&1") // exec merges stderr anyway
	if strings.ContainsAny(strings.NewReplacer("&&", "", ";", "").Replace(body), "|&<>`\n") || strings.Contains(body, "$(") {
		return line, false
	}
	parts, seps := segments(body)
	for _, p := range parts {
		if f := strings.Fields(p); len(f) > 0 && shellKeywords[f[0]] {
			return line, false // loops, conditionals, groups: wrapping a piece could change what runs
		}
	}
	var out strings.Builder
	wrapped := false
	for i, p := range parts {
		if i > 0 {
			out.WriteString(map[string]string{";": "; ", "&&": " && "}[seps[i-1]])
		}
		env, cmd := splitEnv(p)
		if passthrough(cmd) {
			out.WriteString(p)
			continue
		}
		wrapped = true
		run := prefix + " -- "
		if i == len(parts)-1 && slices.Contains(strings.Fields(pipe), "head") {
			run = prefix + " -attr " + stdoutexporter.AttrHint + "=" + stdoutexporter.HintTop + " -- " // head would cut a hint at the end
		}
		out.WriteString(env + run + cmd)
	}
	if !wrapped {
		return line, false
	}
	if merged {
		out.WriteString(" 2>&1")
	}
	return out.String() + pipe, true
}

// shellKeywords start compound commands, whose parts are not separate commands.
var shellKeywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true, "for": true, "while": true, "until": true,
	"do": true, "done": true, "case": true, "esac": true, "select": true, "function": true, "{": true, "}": true,
}

// segments splits a command list at && and ; outside quotes: the trimmed
// commands, and the separator after each but the last.
func segments(s string) (parts, seps []string) {
	var quote byte
	start := 0
	cut := func(end int, sep string) {
		parts = append(parts, strings.TrimSpace(s[start:end]))
		seps = append(seps, sep)
	}
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ';':
			cut(i, ";")
			start = i + 1
		case c == '&' && i+1 < len(s) && s[i+1] == '&':
			cut(i, "&&")
			i++
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts, seps
}

// splitPipe cuts trailing "| head …" and "| tail …" stages off a line; pipe is
// empty when there are none or another command follows a pipe.
func splitPipe(line string) (body, pipe string) {
	stages := strings.Split(line, "|")
	if len(stages) == 1 {
		return line, ""
	}
	for _, s := range stages[1:] {
		args := strings.Fields(s)
		if len(args) == 0 || args[0] != "head" && args[0] != "tail" || streams(s) {
			return line, ""
		}
		for _, a := range args[1:] {
			if strings.Trim(a, "-+ncq0123456789") != "" {
				return line, ""
			}
		}
	}
	body = strings.TrimSpace(stages[0])
	return body, strings.TrimPrefix(line, body)
}

// ownCommand reports lines that run gascoigne itself or ask for raw output.
func ownCommand(line string) bool {
	if strings.Contains(line, "GASCOIGNE_RAW") {
		return true
	}
	for s := range strings.FieldsFuncSeq(line, func(r rune) bool { return strings.ContainsRune("|;&", r) }) {
		if args := detect.Argv(s); len(args) > 0 && args[0] == "gascoigne" {
			return true
		}
	}
	return false
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

// passthrough reports commands exec should not run: shell builtins (cd,
// export: they must change this shell), commands that print little or
// nothing (each would only add a bypassed run to the stats), interactive
// ones and those that run until stopped. Unknown programs are wrapped: their
// output is detected by content (JSON, logs, tables).
func passthrough(cmd string) bool {
	args := detect.Argv(cmd)
	if len(args) == 0 {
		return true
	}
	name := filepath.Base(args[0])
	if quiet[name] || len(args) == 1 && repl[name] {
		return true
	}
	return streams(cmd)
}

var quiet = map[string]bool{
	// builtins
	"cd": true, "pushd": true, "popd": true, "export": true, "unset": true, "set": true, "source": true, ".": true,
	"alias": true, "unalias": true, "exit": true, "return": true, "eval": true, "exec": true, "read": true,
	"wait": true, "trap": true, "ulimit": true, "umask": true, "shift": true, "local": true, "declare": true,
	"typeset": true, "readonly": true, "shopt": true, "hash": true, "type": true, "command": true, "builtin": true,
	"jobs": true, "fg": true, "bg": true, "disown": true, ":": true, "true": true, "false": true, "test": true, "[": true, "[[": true,
	// little output
	"echo": true, "printf": true, "sleep": true, "mkdir": true, "rmdir": true, "rm": true, "mv": true, "cp": true,
	"touch": true, "chmod": true, "chown": true, "ln": true, "which": true, "pwd": true, "clear": true, "kill": true,
	// interactive
	"vi": true, "vim": true, "nvim": true, "nano": true, "emacs": true, "less": true, "more": true, "watch": true,
}

// repl: programs that read commands interactively when run without arguments.
var repl = map[string]bool{"python": true, "python3": true, "node": true, "irb": true, "ghci": true, "psql": true, "mysql": true, "sqlite3": true, "bash": true, "sh": true, "zsh": true}

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

// execPrefix is the rewritten command up to "--". Agents registered for
// good (wrap -print) do not set GASCOIGNE_AGENT and GASCOIGNE_SESSION for
// their shells the way wrap does, so the hook passes what it knows as flags
// and the run is still tagged with its agent and session.
func execPrefix(gascoigne, agent, session string) string {
	out := gascoigne + " exec"
	if os.Getenv("GASCOIGNE_AGENT") == "" {
		out += " -agent " + shellQuote(agent)
	}
	if os.Getenv("GASCOIGNE_SESSION") == "" && session != "" {
		out += " -session " + shellQuote(session)
	}
	return out
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
