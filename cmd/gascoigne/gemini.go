package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/lukaszb/gascoigne/payload"
)

// Gemini CLI: a BeforeTool hook rewrites run_shell_command through
// gascoigne exec, SessionStart adds the usage guide; MCP servers and hooks
// live in .gemini/settings.json. Per run, a system settings file (highest
// precedence) carries them, so nothing the user owns is touched.

func geminiSettings(exe string) map[string]any {
	hook := []any{map[string]any{"type": "command", "command": hookCommand(exe, "gemini"), "name": "gascoigne"}}
	return map[string]any{
		"hooks": map[string]any{
			"BeforeTool":   []any{map[string]any{"matcher": "run_shell_command", "hooks": hook}},
			"AfterTool":    []any{map[string]any{"matcher": "run_shell_command|mcp_.*", "hooks": hook}},
			"SessionStart": []any{map[string]any{"hooks": hook}},
		},
		"mcpServers": mcpServers(exe)["mcpServers"],
	}
}

// geminiHook answers BeforeTool with the rewritten command; Gemini merges
// tool_input over the model's arguments. No decision: approval stays the user's.
var geminiHook = geminiAfter(withGuide(rewriteHook(func(updated map[string]any) any {
	if updated == nil {
		return nil
	}
	return map[string]any{"hookSpecificOutput": map[string]any{"tool_input": map[string]any{"command": updated["command"]}}}
})))

// geminiAfter answers AfterTool: output the BeforeTool rewrite did not
// reach (commands left as they were, MCP servers' results) is optimized,
// and "deny" with a reason replaces the result the model sees.
func geminiAfter(h hookFunc) hookFunc {
	return func(ctx context.Context, c *common, in hookInput, prefix string) (any, error) {
		if in.HookEventName != "AfterTool" {
			return h(ctx, c, in, prefix)
		}
		attrs := map[string]string{payload.AttrTool: in.ToolName}
		if in.ToolName == "run_shell_command" {
			cmd, _ := in.ToolInput["command"].(string)
			if ownCommand(cmd) {
				return nil, nil
			}
			attrs = map[string]string{payload.AttrCmd: cmd}
		} else if !isOtherMCP(in.ToolName) {
			return nil, nil
		}
		text, ok := mcpText(in.ToolResponse)
		if !ok {
			return nil, nil
		}
		out, err := optimizeText(ctx, c, text, attrs)
		if err != nil || out == "" {
			return nil, err
		}
		return map[string]any{"decision": "deny", "reason": out}, nil
	}
}

// geminiSystemSettings is the system settings file Gemini reads, unless
// GEMINI_CLI_SYSTEM_SETTINGS_PATH points elsewhere.
func geminiSystemSettings() string {
	if p := os.Getenv("GEMINI_CLI_SYSTEM_SETTINGS_PATH"); p != "" {
		return p
	}
	switch runtime.GOOS {
	case "darwin":
		return "/Library/Application Support/GeminiCli/settings.json"
	case "windows":
		return `C:\ProgramData\gemini-cli\settings.json`
	}
	return "/etc/gemini-cli/settings.json"
}

func geminiRunFile() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gascoigne", "gemini", "settings.json"), nil
}

// geminiEnv writes the run's system settings: the real system settings
// (they would otherwise be replaced), proxied copies of the user's and the
// project's stdio servers, and gascoigne's hooks and server.
func geminiEnv(exe string) ([]string, error) {
	file, err := geminiRunFile()
	if err != nil {
		return nil, err
	}
	sys := geminiSystemSettings()
	root := readJSONObject(sys)
	if sys == file || root == nil {
		root = toObject(map[string]any{})
	}
	servers := toObject(map[string]any{})
	home, _ := os.UserHomeDir()
	for _, f := range []string{filepath.Join(home, ".gemini", "settings.json"), filepath.Join(".gemini", "settings.json")} {
		if s := obj(path(readJSONObject(f), "mcpServers")); s != nil {
			mergeJSON(servers, s)
		}
	}
	r := proxyServers(exe, servers, nil)
	reportProxies(r)
	removeGeminiSettings(root)
	mergeJSON(root, toObject(map[string]any{"mcpServers": map[string]any{}}))
	for _, name := range r.proxied {
		obj(root.get("mcpServers")).set(name, servers.get(name))
	}
	mergeJSON(root, toObject(geminiSettings(exe)))
	out, err := formatJSON(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(file, out, 0o600); err != nil {
		return nil, fmt.Errorf("write gemini settings: %w", err)
	}
	return []string{"GEMINI_CLI_SYSTEM_SETTINGS_PATH=" + file}, nil
}

func geminiInstall(exe string) string {
	return "// merge into ~/.gemini/settings.json (or .gemini/settings.json in a project)\n" + indentJSON(geminiSettings(exe)) + "\n"
}

func addGemini(exe string, global bool) ([]string, error) {
	file, err := scoped(global, ".gemini/settings.json", ".gemini/settings.json")
	if err != nil {
		return nil, err
	}
	d, err := addJSON(file, removeGeminiSettings, geminiSettings(exe), "BeforeTool and SessionStart hooks, MCP server gascoigne", proxyMCP(exe))
	return []string{d}, err
}

func removeGeminiSettings(root *object) []string {
	what := mcpEdit(root)
	for _, ev := range []string{"BeforeTool", "AfterTool", "SessionStart"} {
		if removeHooks(root, ev, ourHook("gemini")) > 0 {
			what = append(what, ev+" hook")
		}
	}
	return what
}

func removeGemini() ([]string, error) {
	var done []string
	var errs []error
	for _, f := range configFiles(".gemini/settings.json", ".gemini/settings.json") {
		d, err := editJSON(f, removeGeminiSettings)
		done, errs = append(done, d...), append(errs, err)
	}
	if f, err := geminiRunFile(); err == nil {
		if err := os.Remove(f); err == nil {
			done = append(done, display(f)+": deleted run settings")
		} else if !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return done, errors.Join(errs...)
}
