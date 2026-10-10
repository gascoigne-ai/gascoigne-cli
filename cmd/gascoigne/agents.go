package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// agent wires gascoigne into one coding agent: shell output through
// gascoigne exec (or optimized after the fact), the MCP server for web data.
type agent struct {
	bin     string                                          // program wrap starts
	launch  func(exe string) ([]string, error)              // per-run config args; nil: the agent has none
	install func(exe string) string                         // config to register gascoigne for good (wrap -print)
	add     func(exe string, global bool) ([]string, error) // writes that config: user files, else project ones
	remove  func() ([]string, error)                        // undoes install: what it removed
	hook    hookFunc
}

var agents = map[string]agent{
	"claude": {bin: "claude", launch: claudeArgs, install: claudeInstall, add: addClaude, remove: removeClaude,
		hook: rewriteHook(func(updated map[string]any) any {
			if updated == nil {
				return nil
			}
			return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "updatedInput": updated}}
		})},
	// Codex rewrites a command only together with approving it, so its
	// output is optimized after the run instead and replaces the result.
	"codex": {bin: "codex", launch: codexArgs, install: codexInstall, add: addCodex, remove: removeCodex,
		hook: optimizeHook(func(text string) any { return map[string]any{"decision": "block", "reason": text} })},
	// Cursor reads hooks and MCP servers from its config files only.
	"cursor": {install: cursorInstall, add: addCursor, remove: removeCursor,
		hook: rewriteHook(func(updated map[string]any) any {
			if updated == nil {
				return map[string]any{"permission": "allow"}
			}
			return map[string]any{"permission": "allow", "updated_input": updated}
		})},
	"pi": {bin: "pi", launch: piArgs, install: piInstall, add: addPi, remove: removePi,
		hook: rewriteHook(func(updated map[string]any) any {
			if updated == nil {
				return nil
			}
			return map[string]any{"command": updated["command"]}
		})},
}

func agentNames() string {
	names := make([]string, 0, len(agents))
	for n := range agents {
		names = append(names, n)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}

// cmdWrap starts an agent with gascoigne registered for this run only, or
// prints the config that registers it for good.
func cmdWrap(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("wrap", flag.ContinueOnError)
	print := fs.Bool("print", false, "print the config that registers gascoigne with the agent for good, instead of starting it")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "usage: gascoigne wrap [-print] <agent> [agent args...]\nagents: %s\n", agentNames())
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	name := fs.Arg(0)
	a, ok := agents[name]
	if !ok {
		fs.Usage()
		return flag.ErrHelp
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if *print {
		_, err := fmt.Print(a.install(exe))
		return err
	}
	if a.launch == nil {
		return fmt.Errorf("wrap: %s takes no per-run config; register gascoigne once with: gascoigne wrap -print %s", name, name)
	}
	extra, err := a.launch(exe)
	if err != nil {
		return err
	}
	if !serverOff() {
		// The dashboard address goes to stderr before the agent takes the terminal.
		start := exec.Command(exe, "server", "start")
		start.Stdout, start.Stderr = os.Stderr, os.Stderr
		if err := start.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "gascoigne: dashboard not started:", err)
		}
	}
	cmd := exec.Command(a.bin, append(extra, fs.Args()[1:]...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(),
		"GASCOIGNE_AGENT="+cmp.Or(os.Getenv("GASCOIGNE_AGENT"), name),
		"GASCOIGNE_SESSION="+cmp.Or(os.Getenv("GASCOIGNE_SESSION"), name+"-"+time.Now().Format("20060102-150405")))
	err = cmd.Run()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
		return exitCode(exitErr.ExitCode())
	}
	return err
}

func hookCommand(exe, agent string) string { return shellQuote(exe) + " hook " + agent }

func mcpServers(exe string) map[string]any {
	return map[string]any{"mcpServers": map[string]any{"gascoigne": map[string]any{"command": exe, "args": []string{"mcp"}}}}
}

func claudeSettings(exe string) map[string]any {
	return map[string]any{
		"env": map[string]string{"GASCOIGNE_AGENT": "claude"},
		"hooks": map[string]any{"PreToolUse": []any{map[string]any{
			"matcher": "Bash",
			"hooks":   []any{map[string]any{"type": "command", "command": hookCommand(exe, "claude")}},
		}}},
	}
}

func claudeArgs(exe string) ([]string, error) {
	// Hooks cannot shrink built-in tool results: without Grep and Glob, Claude
	// searches with rg and find through Bash, which the hook wraps. Read stays,
	// Edit needs it. Per run only: unwrap could not tell this deny rule from the
	// user's own.
	settings := claudeSettings(exe)
	settings["permissions"] = map[string]any{"deny": []string{"Grep", "Glob"}}
	// --settings ends --mcp-config's list of files, so agent args stay agent args.
	return []string{"--mcp-config", compactJSON(mcpServers(exe)), "--settings", compactJSON(settings)}, nil
}

func claudeInstall(exe string) string {
	return fmt.Sprintf(`# merge into ~/.claude/settings.json (or .claude/settings.json in a project)
%s
# MCP server (fetch, compress, retrieve, stats)
claude mcp add --scope user gascoigne -- %s mcp
# permission rules see the wrapped command: allow e.g. "Bash(gascoigne exec -- go test:*)"
# optional: deny "Grep" and "Glob" so searches run through Bash (rg, find) and get optimized
`, indentJSON(claudeSettings(exe)), shellQuote(exe))
}

func codexArgs(exe string) ([]string, error) {
	fmt.Fprintln(os.Stderr, "gascoigne: on first run, trust the gascoigne hook in codex with /hooks")
	return []string{
		"-c", fmt.Sprintf(`mcp_servers.gascoigne={command=%s,args=["mcp"]}`, strconv.Quote(exe)),
		"-c", fmt.Sprintf(`hooks.PostToolUse=[{matcher="^Bash$",hooks=[{type="command",command=%s}]}]`, strconv.Quote(hookCommand(exe, "codex"))),
	}, nil
}

func codexInstall(exe string) string {
	return "# add to ~/.codex/config.toml, then trust the hook in codex with /hooks\n" + codexTOML(exe)
}

func codexTOML(exe string) string {
	return fmt.Sprintf(`[mcp_servers.gascoigne]
command = %s
args = ["mcp"]

[[hooks.PostToolUse]]
matcher = "^Bash$"

[[hooks.PostToolUse.hooks]]
type = "command"
command = %s
`, strconv.Quote(exe), strconv.Quote(hookCommand(exe, "codex")))
}

func cursorInstall(exe string) string {
	hooks := map[string]any{"version": 1, "hooks": map[string]any{"preToolUse": []any{
		map[string]any{"command": hookCommand(exe, "cursor"), "matcher": "Shell"},
	}}}
	return fmt.Sprintf("# merge into ~/.cursor/hooks.json\n%s\n# merge into ~/.cursor/mcp.json\n%s\n",
		indentJSON(hooks), indentJSON(mcpServers(exe)))
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

func compactJSON(v any) string {
	b, _ := json.Marshal(v) // maps and strings always encode
	return string(b)
}

func indentJSON(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
