package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/component"
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
	SessionID string `json:"session_id"`
	// AgentID is set when a subagent made the call (Claude Code's hook
	// payload, per its docs; not verified against a live capture).
	AgentID        string          `json:"agent_id"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	PermissionMode string          `json:"permission_mode"` // Claude: default, plan, acceptEdits, auto, dontAsk or bypassPermissions
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
	var shared common
	fs := newFlags("hook")
	shared.register(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	agent, ok := agents[name]
	if !ok {
		return fmt.Errorf("hook: want an agent: %s", agentNames())
	}
	var in hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne hook: read event:", err)
		return nil
	}
	if in.HookEventName == "SessionStart" || in.HookEventName == "UserPromptSubmit" {
		in.syncNote = sessionNotes(ctx, in)
	}
	if in.HookEventName == "UserPromptSubmit" {
		return promptHook(ctx, in)
	}
	model := adoptEvent(&shared, name, in)
	if skipHook(in.HookEventName) {
		return nil
	}
	reply, err := agent.hook(ctx, &shared, in, execPrefix(self(), name, hookSession(in), model, lastContext(transcriptEnd(in.TranscriptPath))))
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

// hookSession is the session the ledger records for the event. A subagent
// shares its parent's session_id, so it gets its agent id appended: rerun
// pointers then never lead to output only the other agent saw. Agents whose
// payload has no agent_id keep the plain session id.
func hookSession(in hookInput) string {
	if in.SessionID == "" || in.AgentID == "" {
		return in.SessionID
	}
	return in.SessionID + "#" + in.AgentID
}

// sessionNotes returns the note a session start or prompt event carries: the
// sync note, plus the nudges a session start adds.
func sessionNotes(ctx context.Context, in hookInput) string {
	note := syncNote()
	checkReleaseQuick(ctx)
	if in.HookEventName != "SessionStart" { // a prompt event leaves asking to the session start
		return note
	}
	setup, release := setupNudge(in.Source), releaseNudge(in.Source, tryOpenBrowser)
	notes := []string{note, setup, release, mcpNudge(in.Source, setup != "" || release != ""), learnNudge(ctx, in.Source)}
	return strings.Join(slices.DeleteFunc(notes, func(note string) bool { return note == "" }), " ")
}

// promptHook starts the server when a prompt arrives and prints the context
// nudge, if there is one.
func promptHook(ctx context.Context, in hookInput) error {
	// The server exits when idle; a prompt is where an agent starts
	// talking to its API again, so bring it back first. It is also where a
	// release that appeared mid-session opens its page (the reply stays empty).
	releaseNudge("prompt", tryOpenBrowser)
	if !serverOff() {
		if err := serverUp(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "gascoigne hook:", err)
		}
	}
	note := contextNudge(in.TranscriptPath)
	if note == "" {
		return nil
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "UserPromptSubmit", "additionalContext": note}})
}

// adoptEvent fills the agent, session and model the user did not set from the
// event, and returns the model it took from the transcript ("" when not).
func adoptEvent(shared *common, name string, in hookInput) (model string) {
	if shared.agent == "" && os.Getenv("GASCOIGNE_AGENT") == "" {
		shared.agent = name
	}
	if shared.sessionID() == "" {
		shared.session = hookSession(in)
	}
	if shared.model == "" && os.Getenv("GASCOIGNE_MODEL") == "" {
		model = transcriptModel(in.TranscriptPath)
		shared.model = model
	}
	return model
}

// rewriteHook routes shell tool calls through gascoigne exec before they run;
// reply shapes the agent's answer (updated is nil when nothing changed).
func rewriteHook(reply func(updated map[string]any) any) hookFunc {
	return func(_ context.Context, _ *common, in hookInput, prefix string) (any, error) {
		cmd := strField(in.ToolInput, "command")
		background, _ := in.ToolInput["run_in_background"].(bool)
		// The call's description says what the output is read for: ranked
		// optimizers (search matches, doc sections, JSON items) keep that first.
		desc := strField(in.ToolInput, "description")
		if q := sample.Intent(desc); q != "" {
			prefix += " -attr query=" + mcptool.ShellQuote(q)
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
		cmd := strField(in.ToolInput, "command")
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
	out := &hookreceiver.Output{Text: text, Cmd: cmd}
	c.factories = []component.Factory{hookreceiver.NewFactory(hookreceiver.WithOutput(out))}
	svc, err := c.start(ctx)
	if err != nil {
		return nil, err
	}
	runErr := svc.Run(ctx, "hook")
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
		out += " -agent " + mcptool.ShellQuote(agent)
	}
	if os.Getenv("GASCOIGNE_SESSION") == "" && session != "" {
		out += " -session " + mcptool.ShellQuote(session)
	}
	if model != "" {
		out += " -model " + mcptool.ShellQuote(model)
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
	return mcptool.ShellQuote(exe)
}

func sameFile(first, second string) bool {
	x, err1 := os.Stat(first)
	y, err2 := os.Stat(second)
	return err1 == nil && err2 == nil && os.SameFile(x, y)
}
