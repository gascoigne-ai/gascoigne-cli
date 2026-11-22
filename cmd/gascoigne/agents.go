package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

// agent wires gascoigne into one coding agent: shell output through
// gascoigne exec (or optimized after the fact), the MCP server for web data.
type agent struct {
	bin     string                                                               // program wrap starts
	launch  func(exe string) ([]string, error)                                   // per-run config args
	env     func(exe string) ([]string, error)                                   // per-run config env (NAME=value); neither: the agent has no per-run config
	install func(exe string) string                                              // config to register gascoigne for good (wrap -print)
	add     func(ctx context.Context, exe string, global bool) ([]string, error) // writes that config: user files, else project ones
	// sync is what an automatic update writes when it differs from add: the
	// parts of the install that are safe while the agent runs.
	sync func(exe string, global bool) ([]string, error)
	// addProxy points the agent at the LLM API proxy for good (install -proxy).
	addProxy func(ctx context.Context, global bool) ([]string, error)
	remove   func(ctx context.Context) ([]string, error) // undoes install: what it removed
	hook     hookFunc
}

var agents = map[string]agent{
	"claude": {bin: "claude", launch: claudeArgs, env: func(string) ([]string, error) { return proxyForClaude(), nil },
		install: claudeInstall, add: addClaude, sync: syncClaude, addProxy: addClaudeProxy, remove: removeClaude,
		hook: withResults(withGuide(withAutoAllow(rewriteHook(func(updated map[string]any) any {
			if updated == nil {
				return nil
			}
			return claudePreReply{HookSpecificOutput: claudePreOutput{HookEventName: "PreToolUse", UpdatedInput: updated}}
		}))))},
	// Codex rewrites a command only together with approving it, so its
	// output is optimized after the run instead and replaces the result.
	"codex": {bin: "codex", launch: codexArgs, install: codexInstall, add: addCodex, remove: removeCodex,
		hook: optimizeHook(func(text string) any { return codexReply{Decision: "block", Reason: text} })},
	// Cursor reads hooks and MCP servers from its config files only.
	"cursor": {install: cursorInstall, add: addCursor, remove: removeCursor,
		hook: rewriteHook(func(updated map[string]any) any {
			if updated == nil {
				return cursorReply{Permission: "allow"}
			}
			return cursorReply{Permission: "allow", UpdatedInput: updated}
		})},
	"gemini":   {bin: "gemini", env: geminiEnv, install: geminiInstall, add: addGemini, remove: removeGemini, hook: geminiHook},
	"opencode": {bin: "opencode", env: opencodeEnv, install: opencodeInstall, add: addOpencode, remove: removeOpencode, hook: opencodeHook},
	"pi": {bin: "pi", launch: piArgs, install: piInstall, add: addPi, remove: removePi,
		hook: rewriteHook(func(updated map[string]any) any {
			if updated == nil {
				return nil
			}
			command, _ := updated["command"].(string)
			return piReply{Command: command}
		})},
}

// Hook replies, one struct per agent shape: keys are fixed, built once and
// serialized once. Tool input handed back to the agent stays a map, since the
// keys the hook does not know must reach the agent as they came.

// claudePreReply answers claude's PreToolUse with the rewritten tool input.
type claudePreReply struct {
	HookSpecificOutput claudePreOutput `json:"hookSpecificOutput"`
}

type claudePreOutput struct {
	HookEventName string `json:"hookEventName"`
	// PermissionDecision is "allow" only where auto mode would otherwise deny
	// the rewrite (see withAutoAllow); empty leaves the permission flow alone.
	PermissionDecision string         `json:"permissionDecision,omitempty"`
	UpdatedInput       map[string]any `json:"updatedInput"`
}

// withAutoAllow approves a rewrite in auto mode. Auto mode's classifier
// reviewed the command the model wrote, so a hook that changes it without a
// decision is denied ("a hook changed this call's input after auto mode
// reviewed it") and the model must retry. Allowing skips that review for the
// call, so it is limited to auto mode: other modes keep their own prompts.
// Deny and ask rules still apply.
func withAutoAllow(next hookFunc) hookFunc {
	return func(ctx context.Context, shared *common, in hookInput, prefix string) (any, error) {
		reply, err := next(ctx, shared, in, prefix)
		if r, ok := reply.(claudePreReply); ok && in.PermissionMode == "auto" {
			r.HookSpecificOutput.PermissionDecision = "allow"
			return r, err
		}
		return reply, err
	}
}

// cursorReply approves the call, with the rewritten input when there is one.
type cursorReply struct {
	Permission   string         `json:"permission"`
	UpdatedInput map[string]any `json:"updated_input,omitempty"`
}

// codexReply replaces a result: "block" with the optimized text as the reason.
type codexReply struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// piReply carries the rewritten command.
type piReply struct {
	Command string `json:"command"`
}

func agentNames() string {
	names := make([]string, 0, len(agents))
	for n := range agents {
		names = append(names, n)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func unknownAgent(name string) error {
	if name == "" {
		return fmt.Errorf("want an agent: %s", agentNames())
	}
	return fmt.Errorf("unknown agent %q, want one of: %s", name, agentNames())
}

// cmdWrap starts an agent with gascoigne registered for this run only, or
// prints the config that registers it for good.
func cmdWrap(ctx context.Context, args []string) error {
	fs := newFlags("wrap")
	print := fs.Bool("print", false, "print the config that registers gascoigne with the agent for good, instead of starting it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	a, ok := agents[name]
	if !ok {
		return unknownAgent(name)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if *print {
		_, err := fmt.Print(a.install(exe))
		return err
	}
	if a.launch == nil && a.env == nil {
		return fmt.Errorf("wrap: %s takes no per-run config; register gascoigne once with: gascoigne install %s", name, name)
	}
	extra, env, err := a.runConfig(exe)
	if err != nil {
		return err
	}
	if !serverOff() {
		startDashboard(ctx, exe)
	}
	cmd := exec.CommandContext(ctx, a.bin, append(extra, fs.Args()[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(append(os.Environ(), env...),
		"GASCOIGNE_AGENT="+cmp.Or(os.Getenv("GASCOIGNE_AGENT"), name),
		"GASCOIGNE_SESSION="+cmp.Or(os.Getenv("GASCOIGNE_SESSION"), name+"-"+time.Now().Format("20060102-150405")))
	err = cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitCode(exitErr.ExitCode())
	}
	return err
}

// runConfig is the extra arguments and environment that register gascoigne
// with the agent for one run.
func (a agent) runConfig(exe string) (extra, env []string, err error) {
	if a.launch != nil {
		if extra, err = a.launch(exe); err != nil {
			return nil, nil, err
		}
	}
	if a.env != nil {
		if env, err = a.env(exe); err != nil {
			return nil, nil, err
		}
	}
	return extra, env, nil
}

// startDashboard starts the background server; its address goes to stderr
// before the agent takes the terminal.
func startDashboard(ctx context.Context, exe string) {
	start := exec.CommandContext(ctx, exe, "server", "start")
	start.Stdout, start.Stderr = os.Stderr, os.Stderr
	if err := start.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: dashboard not started:", err)
	}
}

func hookCommand(exe, agent string) string { return mcptool.ShellQuote(exe) + " hook " + agent }

func mcpServers(exe string) map[string]any {
	return map[string]any{"mcpServers": map[string]any{"gascoigne": map[string]any{"command": exe, "args": []string{"mcp"}}}}
}

func claudeSettings(exe string) map[string]any {
	return map[string]any{
		"env": map[string]string{"GASCOIGNE_AGENT": "claude"},
		"hooks": map[string]any{
			"PreToolUse": []any{map[string]any{
				"matcher": "Bash",
				"hooks":   []any{map[string]any{"type": "command", "command": hookCommand(exe, "claude")}},
			}},
			// Optimizes what the rewrite could not reach: Bash left as it was,
			// Grep's content, every MCP server's results, image Reads (downscaled).
			"PostToolUse": []any{map[string]any{
				"matcher": "Bash|Grep|Read|mcp__.*",
				"hooks":   []any{map[string]any{"type": "command", "command": hookCommand(exe, "claude")}},
			}},
			// Starts the background server again before the first request
			// after it exited idle (the LLM proxy lives in it).
			"UserPromptSubmit": []any{map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": hookCommand(exe, "claude")}},
			}},
			// Adds the usage guide at start, and again after compaction or /clear.
			"SessionStart": []any{map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": hookCommand(exe, "claude")}},
			}},
		},
	}
}

func claudeArgs(exe string) ([]string, error) {
	// Hooks cannot shrink built-in tool results: without Grep and Glob, Claude
	// searches with rg and find through Bash, which the hook wraps; without
	// WebFetch it reads pages with gascoigne's fetch, which keeps them whole
	// instead of a small model's retelling. Read stays, Edit needs it;
	// WebSearch stays, gascoigne has no search. Per run only: unwrap could not
	// tell this deny rule from the user's own.
	settings := claudeSettings(exe)
	if denies := deniedTools(); len(denies) > 0 {
		settings["permissions"] = map[string]any{"deny": denies}
	}
	// Same-named servers given with --mcp-config replace the configured ones
	// for the run, so other servers' results get optimized too.
	servers, r := claudeProxies(exe)
	reportProxies(r)
	servers["gascoigne"] = mcpServers(exe)["mcpServers"].(map[string]any)["gascoigne"]
	// --settings ends --mcp-config's list of files, so agent args stay agent args.
	return []string{"--mcp-config", compactJSON(map[string]any{"mcpServers": servers}), "--settings", compactJSON(settings)}, nil
}

func claudeInstall(exe string) string {
	return fmt.Sprintf(`# merge into ~/.claude/settings.json (or .claude/settings.json in a project)
%s
# MCP server (fetch, compress, show, retrieve, stats)
claude mcp add --scope user gascoigne -- %s mcp
# permission rules see the wrapped command: allow e.g. "Bash(gascoigne exec -- go test:*)"
# optional: deny "Grep" and "Glob" so searches run through Bash (rg, find) and get optimized,
# and "WebFetch" so pages are read with the gascoigne fetch tool
`, indentJSON(claudeSettings(exe)), mcptool.ShellQuote(exe))
}

func codexArgs(exe string) ([]string, error) {
	fmt.Fprintln(os.Stderr, "gascoigne: on first run, trust the gascoigne hook in codex with /hooks")
	proxies, r := codexProxyArgs(exe, codexConfigured())
	reportProxies(r)
	if g := guide(true); !codexHasInstructions() && g != "" {
		proxies = append(proxies, "-c", "developer_instructions="+tomlQuote(g))
	}
	proxies = append(proxies, proxyForCodex()...)
	return append([]string{
		"-c", fmt.Sprintf(`mcp_servers.gascoigne={command=%s,args=["mcp"]}`, strconv.Quote(exe)),
		"-c", fmt.Sprintf(`hooks.PostToolUse=[{matcher=%s,hooks=[{type="command",command=%s}]}]`, strconv.Quote(codexMatcher), strconv.Quote(hookCommand(exe, "codex"))),
		"-c", fmt.Sprintf(`hooks.UserPromptSubmit=[{hooks=[{type="command",command=%s}]}]`, strconv.Quote(hookCommand(exe, "codex"))),
	}, proxies...), nil
}

// codexConfigured are the MCP servers of the user's and the project's codex
// config; a project table replaces the user's of the same name.
func codexConfigured() []codexServer {
	user := ".codex/config.toml"
	files := configFiles(user, ".codex/config.toml")
	if home := os.Getenv("CODEX_HOME"); home != "" {
		files[0] = filepath.Join(home, "config.toml")
	}
	var out []codexServer
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, s := range codexServers(string(data)) {
			out = slices.DeleteFunc(out, func(existing codexServer) bool { return existing.name == s.name })
			out = append(out, s)
		}
	}
	return out
}

// codexHasInstructions: the user set developer_instructions, which a -c
// would replace for the run.
func codexHasInstructions() bool {
	data, err := os.ReadFile(filepath.Join(codexHome(), "config.toml"))
	if err != nil {
		return false
	}
	_, _, _, ok := tomlKey(tomlSections(string(data))[0].body, "developer_instructions")
	return ok
}

func reportProxies(result proxyResult) {
	if s := result.String(); s != "" {
		fmt.Fprintln(os.Stderr, "gascoigne:", s)
	}
}

func codexInstall(exe string) string {
	return "# add to ~/.codex/config.toml, then trust the hook in codex with /hooks\n" + codexTOML(exe)
}

func codexTOML(exe string) string {
	return fmt.Sprintf(`[mcp_servers.gascoigne]
command = %s
args = ["mcp"]

[[hooks.PostToolUse]]
matcher = %s

[[hooks.PostToolUse.hooks]]
type = "command"
command = %s

[[hooks.UserPromptSubmit]]

[[hooks.UserPromptSubmit.hooks]]
type = "command"
command = %s
`, strconv.Quote(exe), strconv.Quote(codexMatcher), strconv.Quote(hookCommand(exe, "codex")), strconv.Quote(hookCommand(exe, "codex")))
}

// codexMatcher: shell commands and every MCP server's tools (the hook
// skips gascoigne's own).
const codexMatcher = "^Bash$|^mcp__.*"

func cursorInstall(exe string) string {
	return fmt.Sprintf("# merge into ~/.cursor/hooks.json\n%s\n# merge into ~/.cursor/mcp.json\n%s\n",
		indentJSON(cursorHooks(exe)), indentJSON(mcpServers(exe)))
}

// piExtension rewrites bash tool calls through gascoigne hook pi; pi has no MCP.
func piExtension(exe string) string {
	return fmt.Sprintf(piHeader+` through "gascoigne exec" (gascoigne wrap pi).
import { execFileSync } from "node:child_process";

process.env.GASCOIGNE_AGENT ??= "pi";

export default function (pi: any) {
  pi.on("tool_call", (event: any) => {
    if (event.toolName !== "bash") return;
    try {
      const out = execFileSync(%s, ["hook", "pi"], { input: JSON.stringify({ tool_input: event.input }) }).toString();
      if (out.trim()) event.input.command = JSON.parse(out).command;
    } catch {}
  });
}
`, compactJSON(exe))
}

func piArgs(exe string) ([]string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "gascoigne", "pi", "gascoigne.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(piExtension(exe)), 0o644); err != nil {
		return nil, fmt.Errorf("write pi extension: %w", err)
	}
	return []string{"--extension", path}, nil
}

func piInstall(exe string) string {
	return "// save as ~/.pi/agent/extensions/gascoigne.ts\n" + piExtension(exe)
}

func compactJSON(value any) string {
	b, _ := json.Marshal(value) // maps and strings always encode
	return string(b)
}

func indentJSON(value any) string {
	b, _ := json.MarshalIndent(value, "", "  ")
	return string(b)
}
