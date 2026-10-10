package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// opencode: a plugin rewrites bash tool calls through gascoigne exec (its
// tool.execute.before hook may change the arguments); MCP servers live
// under "mcp" in opencode.json, as {type: local, command: [...]}. Per run,
// OPENCODE_CONFIG_DIR brings the plugin and OPENCODE_CONFIG_CONTENT the
// servers, both merged over the user's config.

const opencodeHeader = "// gascoigne: routes bash tool calls"

func opencodePlugin(exe string) string {
	return fmt.Sprintf(opencodeHeader+` through "gascoigne exec" (gascoigne install opencode).
import { execFileSync } from "node:child_process";

export const Gascoigne = async () => {
  process.env.GASCOIGNE_AGENT ??= "opencode";
  return {
    "tool.execute.before": async (input: any, output: any) => {
      if (input.tool !== "bash" || typeof output.args?.command !== "string") return;
      try {
        const event = { session_id: input.sessionID, tool_input: output.args };
        const out = execFileSync(%s, ["hook", "opencode"], { input: JSON.stringify(event) }).toString();
        if (out.trim()) output.args.command = JSON.parse(out).command;
      } catch {}
    },
  };
};
`, compactJSON(exe))
}

var opencodeHook = rewriteHook(func(updated map[string]any) any {
	if updated == nil {
		return nil
	}
	return map[string]any{"command": updated["command"]}
})

func opencodeServer(exe string) map[string]any {
	return map[string]any{"mcp": map[string]any{"gascoigne": map[string]any{"type": "local", "command": []string{exe, "mcp"}}}}
}

// opencodeHome is opencode's global config directory.
func opencodeHome() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

// opencodeEnv writes the plugin and the guide to the user cache and points
// opencode at them for this run, with gascoigne's and proxied servers.
func opencodeEnv(exe string) ([]string, error) {
	if os.Getenv("OPENCODE_CONFIG_DIR") != "" {
		return nil, errors.New("wrap opencode: OPENCODE_CONFIG_DIR is set; register for good with: gascoigne install opencode")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(cache, "gascoigne", "opencode")
	plugin := filepath.Join(dir, "plugins", "gascoigne.ts")
	if err := os.MkdirAll(filepath.Dir(plugin), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(plugin, []byte(opencodePlugin(exe)), 0o644); err != nil {
		return nil, fmt.Errorf("write opencode plugin: %w", err)
	}
	cfg := toObject(opencodeServer(exe))
	servers := toObject(map[string]any{})
	files := []string{filepath.Join(opencodeHome(), "opencode.json"), "opencode.json"}
	hasInstructions := false
	for _, f := range files {
		o := readJSONObject(f)
		if s := obj(path(o, "mcp")); s != nil {
			mergeJSON(servers, s)
		}
		hasInstructions = hasInstructions || path(o, "instructions") != nil
	}
	r := proxyOpencode(exe, servers)
	reportProxies(r)
	for _, name := range r.proxied {
		obj(cfg.get("mcp")).set(name, servers.get(name))
	}
	if providers := proxyForOpencode(); providers != nil {
		cfg.set("provider", toObject(providers))
	}
	if !hasInstructions { // a list given here would replace the user's for the run
		guideFile := filepath.Join(dir, "gascoigne.md")
		if err := os.WriteFile(guideFile, []byte(guide(true)), 0o644); err != nil {
			return nil, err
		}
		cfg.set("instructions", []any{guideFile})
	}
	out, err := formatJSON(cfg)
	if err != nil {
		return nil, err
	}
	return []string{"OPENCODE_CONFIG_DIR=" + dir, "OPENCODE_CONFIG_CONTENT=" + string(out)}, nil
}

// proxyOpencode rewrites local servers of an "mcp" object to start through
// the proxy; remote ones stay direct.
func proxyOpencode(exe string, servers *object) proxyResult {
	var r proxyResult
	if servers == nil {
		return r
	}
	for _, name := range servers.keys {
		e, ok := servers.get(name).(*object)
		if !ok || name == "gascoigne" {
			continue
		}
		cmd, ok := stringsOf(e.get("command"))
		switch {
		case e.get("type") == "remote" || e.get("url") != nil:
			r.direct = append(r.direct, name+" (remote: only local servers can be proxied)")
		case !ok || len(cmd) == 0:
			r.direct = append(r.direct, name+" (no command)")
		default:
			if _, done := proxiedArgs(cmd[0], cmd[1:]); !done {
				c, args := proxyCommand(exe, cmd[0], cmd[1:])
				e.set("command", anys(append([]string{c}, args...)))
			}
			r.proxied = append(r.proxied, name)
		}
	}
	return r
}

func unproxyOpencode(servers *object) []string {
	var names []string
	if servers == nil {
		return nil
	}
	for _, name := range servers.keys {
		e, ok := servers.get(name).(*object)
		if !ok {
			continue
		}
		cmd, _ := stringsOf(e.get("command"))
		if len(cmd) == 0 {
			continue
		}
		if orig, ok := proxiedArgs(cmd[0], cmd[1:]); ok {
			e.set("command", anys(orig))
			names = append(names, name)
		}
	}
	return names
}

func removeOpencodeConfig(root *object) []string {
	var what []string
	servers := obj(root.get("mcp"))
	if names := unproxyOpencode(servers); len(names) > 0 {
		what = append(what, "proxy from MCP servers "+strings.Join(names, ", "))
	}
	if cmd, _ := stringsOf(path(servers, "gascoigne", "command")); len(cmd) > 0 && strings.Contains(cmd[0], "gascoigne") {
		servers.del("gascoigne")
		pruneEmpty(root, "mcp")
		what = append(what, "MCP server gascoigne")
	}
	return what
}

func opencodeInstall(exe string) string {
	return "// merge into ~/.config/opencode/opencode.json (or opencode.json in a project)\n" + indentJSON(opencodeServer(exe)) +
		"\n// save as ~/.config/opencode/plugins/gascoigne.ts (or .opencode/plugins/gascoigne.ts)\n" + opencodePlugin(exe)
}

func addOpencode(exe string, global bool) ([]string, error) {
	cfgFile, plugin, agentsMD := "opencode.json", filepath.Join(".opencode", "plugins", "gascoigne.ts"), "AGENTS.md"
	if global {
		home := opencodeHome()
		cfgFile, plugin, agentsMD = filepath.Join(home, "opencode.json"), filepath.Join(home, "plugins", "gascoigne.ts"), filepath.Join(home, "AGENTS.md")
	}
	also := func(root *object) proxyResult { return proxyOpencode(exe, obj(root.get("mcp"))) }
	d, err := addJSON(cfgFile, removeOpencodeConfig, opencodeServer(exe), "MCP server gascoigne", also)
	if err != nil {
		return nil, err
	}
	done := []string{d}
	_, err = writeConfig(plugin, func(text string) (string, error) {
		if text != "" && !strings.HasPrefix(text, opencodeHeader) {
			return "", errors.New("exists and was not written by gascoigne, left alone")
		}
		return opencodePlugin(exe), nil
	})
	if err != nil && !errors.Is(err, errUnchanged) {
		return done, err
	}
	done = append(done, added(plugin, "plugin", "", err))
	d, err = addGuide(agentsMD, true)
	return append(done, d), err
}

func removeOpencode() ([]string, error) {
	var done []string
	var errs []error
	for _, f := range []string{filepath.Join(opencodeHome(), "opencode.json"), "opencode.json"} {
		d, err := editJSON(f, removeOpencodeConfig)
		done, errs = append(done, d...), append(errs, err)
	}
	plugins := []string{filepath.Join(opencodeHome(), "plugins", "gascoigne.ts"), filepath.Join(".opencode", "plugins", "gascoigne.ts")}
	if cache, err := os.UserCacheDir(); err == nil {
		plugins = append(plugins, filepath.Join(cache, "gascoigne", "opencode", "plugins", "gascoigne.ts"))
	}
	for _, f := range slices.Compact(plugins) {
		data, err := os.ReadFile(f)
		if err != nil || !strings.HasPrefix(string(data), opencodeHeader) {
			continue
		}
		if err := os.Remove(f); err != nil {
			errs = append(errs, err)
			continue
		}
		done = append(done, display(f)+": deleted plugin")
	}
	for _, f := range []string{filepath.Join(opencodeHome(), "AGENTS.md"), "AGENTS.md"} {
		d, err := removeGuide(f)
		done, errs = append(done, d...), append(errs, err)
	}
	return done, errors.Join(errs...)
}
