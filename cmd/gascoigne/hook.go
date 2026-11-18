package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/internal/hint"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/hookreceiver"
)

// hookInput is the event agent hooks send on stdin. Claude Code, Codex and
// Cursor share these fields; the pi extension sends the same shape.
type hookInput struct {
	SessionID      string          `json:"session_id"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	TranscriptPath string          `json:"transcript_path"`
	Source         string          `json:"source"` // SessionStart: startup, resume, clear or compact
	ToolInput      map[string]any  `json:"tool_input"`
	ToolResponse   json.RawMessage `json:"tool_response"`

	// syncNote tells the agent its gascoigne hooks were just updated; set by
	// cmdHook, not part of the event.
	syncNote string
}

// hookFunc answers one hook event; nil means "leave the tool call alone".
// prefix is how the agent's shell runs a command through this binary
// ("gascoigne exec", see execPrefix).
type hookFunc func(ctx context.Context, shared *common, in hookInput, prefix string) (any, error)

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
	if in.HookEventName == "SessionStart" || in.HookEventName == "UserPromptSubmit" {
		in.syncNote = syncNote()
		checkReleaseQuick(ctx)
		if in.HookEventName == "SessionStart" { // a prompt event leaves asking to the session start
			setup, release := setupNudge(in.Source), releaseNudge(in.Source)
			notes := []string{in.syncNote, setup, release, mcpNudge(in.Source, setup != "" || release != ""), learnNudge(ctx, in.Source)}
			in.syncNote = strings.Join(slices.DeleteFunc(notes, func(note string) bool { return note == "" }), " ")
		}
	}
	if in.HookEventName == "UserPromptSubmit" {
		// The server exits when idle; a prompt is where an agent starts
		// talking to its API again, so bring it back first. It is also where a
		// release that appeared mid-session opens its page (the reply stays empty).
		releaseNudge("prompt")
		if !serverOff() {
			if err := serverUp(); err != nil {
				fmt.Fprintln(os.Stderr, "gascoigne hook:", err)
			}
		}
		note := contextNudge(in.TranscriptPath)
		if note == "" {
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "UserPromptSubmit", "additionalContext": note}})
	}
	if c.agent == "" && os.Getenv("GASCOIGNE_AGENT") == "" {
		c.agent = name
	}
	if c.sessionID() == "" {
		c.session = in.SessionID
	}
	model := ""
	if c.model == "" && os.Getenv("GASCOIGNE_MODEL") == "" {
		model = transcriptModel(in.TranscriptPath)
		c.model = model
	}
	if skipHook(in.HookEventName) {
		return nil
	}
	reply, err := a.hook(ctx, &c, in, execPrefix(self(), name, in.SessionID, model, lastContext(transcriptEnd(in.TranscriptPath))))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne hook:", err)
		return nil
	}
	if reply == nil {
		return nil
	}
	data, err := encodeJSON(reply)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}

// rewriteHook routes shell tool calls through gascoigne exec before they run;
// reply shapes the agent's answer (updated is nil when nothing changed).
func rewriteHook(reply func(updated map[string]any) any) hookFunc {
	return func(_ context.Context, _ *common, in hookInput, prefix string) (any, error) {
		cmd, _ := in.ToolInput["command"].(string)
		background, _ := in.ToolInput["run_in_background"].(bool)
		// The call's description says what the output is read for: ranked
		// optimizers (search matches, doc sections, JSON items) keep that first.
		desc, _ := in.ToolInput["description"].(string)
		if q := sample.Intent(desc); q != "" {
			prefix += " -attr query=" + shellQuote(q)
		}
		wrapped, ok := wrapCommand(cmd, prefix)
		if !ok || background && !strings.Contains(wrapped, " -watch -- ") {
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
	return func(ctx context.Context, shared *common, in hookInput, _ string) (any, error) {
		if isOtherMCP(in.ToolName) {
			text, ok := mcpText(in.ToolResponse)
			if !ok {
				return nil, nil
			}
			out, err := optimizeText(ctx, shared, text, mcptool.Attrs(in.ToolName, in.ToolInput))
			if err != nil || out == "" {
				return nil, err
			}
			return reply(out), nil
		}
		cmd, _ := in.ToolInput["command"].(string)
		text, code, ok := shellOutput(in.ToolResponse)
		if !ok || ownCommand(cmd) {
			return nil, nil
		}
		p, err := shared.optimize(ctx, text, cmd)
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
	c.server, c.useServer, c.project, c.pipeline = true, true, true, "hook"
	svc, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	out := &hookreceiver.Output{Text: text, Cmd: cmd}
	runErr := svc.Run(hookreceiver.With(ctx, out), "hook")
	return out.Result, errors.Join(runErr, svc.Shutdown(context.WithoutCancel(ctx)))
}

// hinted is the optimized text plus how to get the original, as exec prints it.
func hinted(data *payload.Payload) string {
	text := data.Text()
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + hint.Of(data.Attrs[payload.AttrRawID]) + "\n"
}

// execPrefix is the rewritten command up to "--". Agents registered for
// good (wrap -print) do not set GASCOIGNE_AGENT and GASCOIGNE_SESSION for
// their shells the way wrap does, so the hook passes what it knows as flags
// and the run is still tagged with its agent and session.
func execPrefix(gascoigne, agent, session, model string, context int) string {
	out := gascoigne + " exec"
	if os.Getenv("GASCOIGNE_AGENT") == "" {
		out += " -agent " + shellQuote(agent)
	}
	if os.Getenv("GASCOIGNE_SESSION") == "" && session != "" {
		out += " -session " + shellQuote(session)
	}
	if model != "" {
		out += " -model " + shellQuote(model)
	}
	if context >= tightContextTokens {
		out += tightBudgets
	}
	return out
}

// tightContextTokens is the context size from which output budgets shrink:
// every token a hook result adds is re-read on each later turn. The hint still
// points to the original.
const tightContextTokens = 150_000

// tightBudgets: head and tail shrink with max_tokens, which must stay above their sum.
const tightBudgets = " -set optimizers.truncate.max_tokens=4000 -set optimizers.truncate.head_tokens=1300" +
	" -set optimizers.truncate.tail_tokens=1300 -set optimizers.log.max_lines=100 -set optimizers.grep.max_files=40"

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

func sameFile(first, second string) bool {
	x, err1 := os.Stat(first)
	y, err2 := os.Stat(second)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}

// shellQuote quotes s for sh when it holds anything beyond plain path characters.
func shellQuote(text string) string {
	if text != "" && strings.Trim(text, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-./:@+%,=") == "" {
		return text
	}
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}
