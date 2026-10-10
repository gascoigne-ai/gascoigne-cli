package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cmdInstall registers gascoigne with agents for good: in the project's
// config files, or with -g in the user's. Earlier gascoigne entries are
// replaced, so running it again refreshes them; unwrap removes them.
func cmdInstall(_ context.Context, args []string) error {
	fset := newFlags("install")
	global := fset.Bool("g", false, "register for every project (user config) instead of this one")
	if err := fset.Parse(args); err != nil {
		return err
	}
	names := fset.Args()
	if len(names) == 0 {
		names = foundAgents()
	}
	if len(names) == 0 {
		return fmt.Errorf("install: no agent found on PATH; name one of: %s", agentNames())
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := installedName(exe)
	var errs []error
	for _, name := range names {
		a, ok := agents[name]
		if !ok {
			return unknownAgent(name)
		}
		done, err := a.add(cmd, *global)
		errs = append(errs, err)
		for _, d := range done {
			fmt.Printf("%s: %s\n", name, d)
		}
	}
	return errors.Join(errs...)
}

// foundAgents are the agents whose program PATH finds.
func foundAgents() []string {
	var names []string
	for _, name := range strings.Split(agentNames(), ", ") {
		bins := []string{agents[name].bin}
		if name == "cursor" {
			bins = []string{"cursor", "cursor-agent"}
		}
		for _, b := range bins {
			if _, err := exec.LookPath(b); err == nil {
				names = append(names, name)
				break
			}
		}
	}
	return names
}

// installedName is how configs call gascoigne: its name when PATH finds
// this binary (configs stay valid if it moves, and project files can be
// shared), else its path.
func installedName(exe string) string {
	if p, err := exec.LookPath("gascoigne"); err == nil && sameFile(p, exe) {
		return "gascoigne"
	}
	return exe
}

// scoped picks the user (home-relative) or project (cwd-relative) path.
func scoped(global bool, user, project string) (string, error) {
	if !global {
		return project, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, user), nil
}

func addClaude(exe string, global bool) ([]string, error) {
	settings, err := scoped(global, ".claude/settings.json", ".claude/settings.json")
	if err != nil {
		return nil, err
	}
	var done []string
	d, err := addJSON(settings, removeClaudeSettings, claudeSettings(exe), "PreToolUse hook, GASCOIGNE_AGENT env", nil)
	if err != nil {
		return nil, err
	}
	done = append(done, d)
	if d, err := proxyClaudeState(exe, global); err != nil {
		return done, err
	} else if d != "" {
		done = append(done, d)
	}
	if !global {
		d, err := addJSON(".mcp.json", mcpEdit, mcpServers(exe), "MCP server gascoigne (approve it in claude on first use)", proxyMCP(exe))
		return append(done, d), err
	}
	// User MCP servers live in ~/.claude.json, which Claude Code rewrites
	// while it runs: let its CLI do the edit when there is one.
	if _, err := exec.LookPath("claude"); err != nil {
		d, err := addJSON(claudeJSON(), mcpEdit, mcpServers(exe), "MCP server gascoigne", nil)
		return append(done, d), err
	}
	_ = exec.Command("claude", "mcp", "remove", "--scope", "user", "gascoigne").Run() // absent is fine
	if out, err := exec.Command("claude", "mcp", "add", "--scope", "user", "gascoigne", "--", exe, "mcp").CombinedOutput(); err != nil {
		return done, fmt.Errorf("claude mcp add: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return append(done, "claude mcp: added MCP server gascoigne (user scope)"), nil
}

func addCursor(exe string, global bool) ([]string, error) {
	hooksFile, err := scoped(global, ".cursor/hooks.json", ".cursor/hooks.json")
	if err != nil {
		return nil, err
	}
	hooks := map[string]any{"version": 1, "hooks": map[string]any{"preToolUse": []any{
		map[string]any{"command": hookCommand(exe, "cursor"), "matcher": "Shell"},
	}}}
	d1, err := addJSON(hooksFile, removeCursorHooks, hooks, "preToolUse hook", nil)
	if err != nil {
		return nil, err
	}
	d2, err := addJSON(filepath.Join(filepath.Dir(hooksFile), "mcp.json"), mcpEdit, mcpServers(exe), "MCP server gascoigne", proxyMCP(exe))
	if err != nil {
		return []string{d1}, err
	}
	done := []string{d1, d2}
	if global {
		// Cursor keeps user rules in its settings, not in a file.
		return append(done, "usage guide: add it under Cursor Settings → Rules, or run gascoigne install cursor in a project"), nil
	}
	_, err = writeConfig(cursorRuleFile, func(text string) (string, error) {
		if text != "" && !strings.HasPrefix(text, cursorRuleHeader) {
			return "", errors.New("exists and was not written by gascoigne, left alone")
		}
		return cursorRule(), nil
	})
	if err != nil && !errors.Is(err, errUnchanged) {
		return done, err
	}
	return append(done, added(cursorRuleFile, "usage guide rule", "", err)), nil
}

func addCodex(exe string, global bool) ([]string, error) {
	file, err := scoped(global, ".codex/config.toml", ".codex/config.toml")
	if err != nil {
		return nil, err
	}
	if home := os.Getenv("CODEX_HOME"); global && home != "" {
		file = filepath.Join(home, "config.toml")
	}
	var r proxyResult
	backup, err := writeConfig(file, func(text string) (string, error) {
		text, _ = removeCodexTOML(text)
		text, r = proxyCodexTOML(exe, text)
		if text = strings.TrimRight(text, "\n"); text != "" {
			text += "\n\n"
		}
		return text + codexTOML(exe), nil
	})
	if err != nil && !errors.Is(err, errUnchanged) {
		return nil, err
	}
	done := []string{withProxies(added(file, "MCP server gascoigne, PostToolUse hook (trust it in codex with /hooks)", backup, err), r)}
	agentsMD := "AGENTS.md"
	if global {
		agentsMD = filepath.Join(codexHome(), "AGENTS.md")
	}
	d, err := addGuide(agentsMD, true)
	return append(done, d), err
}

func addPi(exe string, global bool) ([]string, error) {
	file, err := scoped(global, ".pi/agent/extensions/gascoigne.ts", ".pi/extensions/gascoigne.ts")
	if err != nil {
		return nil, err
	}
	_, err = writeConfig(file, func(text string) (string, error) {
		if text != "" && !strings.HasPrefix(text, piHeader) {
			return "", errors.New("exists and was not written by gascoigne, left alone")
		}
		return piExtension(exe), nil
	})
	done := []string{display(file) + ": wrote extension"}
	if errors.Is(err, errUnchanged) {
		done = []string{display(file) + ": already registered"}
	} else if err != nil {
		return nil, err
	}
	agentsMD, err := scoped(global, ".pi/agent/AGENTS.md", "AGENTS.md")
	if err != nil {
		return done, err
	}
	d, err := addGuide(agentsMD, true)
	return append(done, d), err
}

// addJSON replaces gascoigne's entries in a JSON config file with add,
// merged into what is there: objects merge, lists append.
func addJSON(file string, remove func(*object) []string, add map[string]any, what string, also func(*object) proxyResult) (string, error) {
	var r proxyResult
	backup, err := writeConfig(file, func(text string) (string, error) {
		// A new file starts as add itself, so removing and re-adding gives
		// the same key order as on any later run.
		root := toObject(add)
		if strings.TrimSpace(text) != "" {
			v, err := parseJSON([]byte(text))
			o, ok := v.(*object)
			if err != nil || !ok {
				return "", fmt.Errorf("not a JSON object, edit it by hand: %v", err)
			}
			root = o
		}
		remove(root)
		mergeJSON(root, toObject(add))
		if also != nil {
			r = also(root)
		}
		out, err := formatJSON(root)
		return string(out), err
	})
	if err != nil && !errors.Is(err, errUnchanged) {
		return "", err
	}
	return withProxies(added(file, what, backup, err), r), nil
}

func withProxies(line string, r proxyResult) string {
	if s := r.String(); s != "" {
		return line + "; " + s
	}
	return line
}

// proxyMCP rewrites an mcpServers object at the root of a config.
func proxyMCP(exe string) func(*object) proxyResult {
	return func(root *object) proxyResult { return proxyServers(exe, obj(root.get("mcpServers")), nil) }
}

// proxyClaudeState proxies servers in ~/.claude.json: the user's (global)
// or this project's local ones. The file is left alone when it is missing
// or nothing changes. A running Claude Code may write its own copy back:
// install with no session open.
func proxyClaudeState(exe string, global bool) (string, error) {
	file := claudeJSON()
	if _, err := os.Stat(file); err != nil {
		return "", nil
	}
	var r proxyResult
	backup, err := writeConfig(file, func(text string) (string, error) {
		root, err := parseJSON([]byte(text))
		if err != nil || obj(root) == nil {
			return "", fmt.Errorf("not a JSON object: %v", err)
		}
		servers := obj(path(root, "mcpServers"))
		if !global {
			cwd, _ := os.Getwd()
			servers = obj(path(root, "projects", cwd, "mcpServers"))
		}
		r = proxyServers(exe, servers, nil)
		if len(r.proxied) == 0 {
			return text, nil
		}
		out, err := formatJSON(root)
		return string(out), err
	})
	if errors.Is(err, errUnchanged) || err == nil && r.String() == "" {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	line := display(file) + ": " + r.String()
	if backup != "" {
		line += " (backup: " + filepath.Base(backup) + ")"
	}
	return line, nil
}

func toObject(m map[string]any) *object {
	v, _ := parseJSON([]byte(compactJSON(m))) // our own maps always parse
	return v.(*object)
}

func mergeJSON(dst, src *object) {
	for _, k := range src.keys {
		sv := src.get(k)
		switch dv := dst.get(k).(type) {
		case *object:
			if so, ok := sv.(*object); ok {
				mergeJSON(dv, so)
				continue
			}
		case []any:
			if sl, ok := sv.([]any); ok {
				dst.set(k, append(dv, sl...))
				continue
			}
		}
		dst.set(k, sv)
	}
}

// errUnchanged: the config already holds what install would write.
var errUnchanged = errors.New("unchanged")

// writeConfig rewrites file (created when missing) with edit. A file that
// changes is kept next to it; the backup name is returned then.
func writeConfig(file string, edit func(text string) (string, error)) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	exists := err == nil
	out, err := edit(string(data))
	if err != nil {
		return "", fmt.Errorf("%s: %w", display(file), err)
	}
	if exists && out == string(data) {
		return "", errUnchanged
	}
	perm := os.FileMode(0o644)
	backup := ""
	if exists {
		if info, err := os.Stat(file); err == nil {
			perm = info.Mode().Perm()
		}
		backup = file + ".gascoigne-bak"
		if err := os.WriteFile(backup, data, perm); err != nil {
			return "", fmt.Errorf("back up %s: %w", file, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(file, []byte(out), perm); err != nil {
		return "", fmt.Errorf("write %s: %w", file, err)
	}
	return backup, nil
}

// added reports a writeConfig result; err is nil or errUnchanged.
func added(file, what, backup string, err error) string {
	if err != nil {
		return display(file) + ": already registered"
	}
	s := display(file) + ": added " + what
	if backup != "" {
		s += " (backup: " + filepath.Base(backup) + ")"
	}
	return s
}
