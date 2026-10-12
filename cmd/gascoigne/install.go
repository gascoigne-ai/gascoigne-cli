package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// cmdInstall registers gascoigne with agents for good: in the project's
// config files, or with -g in the user's. Earlier gascoigne entries are
// replaced, so running it again refreshes them; unwrap removes them.
func cmdInstall(_ context.Context, args []string) error {
	fset := newFlags("install")
	global := fset.Bool("g", false, "register for every project (user config) instead of this one")
	proxy := fset.Bool("proxy", false, "also route the agent's LLM API traffic through the background server's LLM proxy (claude); a stopped server stops the agent until its SessionStart hook starts it")
	allow := fset.String("permissions", "", "which gascoigne tools claude runs without asking: none, local (retrieve, show, stats, compress) or all (adds fetch, which reaches the network) (saved; default none)")
	auto := fset.String("autosync", "", "let agent hooks update user-scope installs after a gascoigne upgrade: "+strings.Join(autosyncChoices, ", ")+" (saved; default: not decided, nothing is updated)")
	doSync := fset.Bool("sync", false, "bring installs made by an older gascoigne up to date now (user scope and this project) and stop")
	level := fset.String("guide", "", "how much usage text agents get: "+strings.Join(guideLevels, ", ")+" (saved; default full)")
	if err := fset.Parse(args); err != nil {
		return err
	}
	if err := errors.Join(saveGuideLevel(*level), saveAllowLevel(*allow), saveAutosync(*auto)); err != nil {
		return err
	}
	if *doSync {
		lines := syncInstalls(true)
		for _, l := range lines {
			fmt.Println(l)
		}
		if len(lines) == 0 {
			fmt.Println("every recorded install is current")
		}
		return nil
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
		if *proxy && a.addProxy != nil && err == nil {
			d, err := a.addProxy(*global)
			done, errs = append(done, d...), append(errs, err)
		} else if *proxy {
			done = append(done, "LLM proxy: not available for this agent; gascoigne wrap "+name+" uses it per run where it can")
		}
		for _, d := range done {
			fmt.Printf("%s: %s\n", name, d)
		}
		if err == nil {
			errs = append(errs, recordInstall(name, *global, cmd, *proxy && a.addProxy != nil))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	offerSetup(os.Stdout)
	return nil
}

// savePref stores the value of an install flag in the prefs before the
// configs that depend on it are written; "" keeps the saved one.
func savePref(flag, value string, valid []string, set func(p *prefs, v string)) error {
	if value == "" {
		return nil
	}
	if !slices.Contains(valid, value) {
		return fmt.Errorf("install: -%s wants one of %s", flag, strings.Join(valid, ", "))
	}
	p := readPrefs()
	set(&p, value)
	return p.save()
}

func saveGuideLevel(v string) error {
	return savePref("guide", v, guideLevels, func(p *prefs, v string) { p.Guide = v })
}

func saveAutosync(v string) error {
	return savePref("autosync", v, autosyncChoices, func(p *prefs, v string) { p.Autosync = v })
}

func saveAllowLevel(v string) error {
	return savePref("permissions", v, allowLevels, func(p *prefs, v string) { p.Allow = v })
}

// autosyncChoices are the answers to "update installed hooks after an upgrade".
var autosyncChoices = []string{"on", "off"}

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

// addClaude writes gascoigne's claude registration: settings, MCP server.
func addClaude(exe string, global bool) ([]string, error) {
	done, err := syncClaude(exe, global)
	if err != nil {
		return nil, err
	}
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

// syncClaude writes the settings file part of the registration: hooks, env,
// deny and allow rules. It is what an automatic update runs, since Claude
// Code reads settings at start but rewrites ~/.claude.json while it runs.
func syncClaude(exe string, global bool) ([]string, error) {
	settings, err := scoped(global, ".claude/settings.json", ".claude/settings.json")
	if err != nil {
		return nil, err
	}
	var done []string
	var denied, allowed []string
	rules := func(root *object) proxyResult {
		denied = denyRules.add(root, claudeDenies)
		// The allow level may have changed since the last install: drop what
		// was recorded, then add what the level has now.
		allowRules.remove(settings, root)
		allowed = allowRules.add(root, claudeAllows(readPrefs().Allow))
		return proxyResult{}
	}
	d, err := addJSON(settings, removeClaudeSettings, claudeSettings(exe), "PreToolUse, PostToolUse and SessionStart hooks, GASCOIGNE_AGENT env", rules)
	if err != nil {
		return nil, err
	}
	if len(denied) > 0 {
		d += "; denied " + strings.Join(denied, ", ") + " (searches through Bash, pages through gascoigne fetch)"
	}
	if len(allowed) > 0 {
		d += "; allowed " + strings.Join(allowed, ", ") + " without asking"
	}
	if err := denyRules.record(settings, denied); err != nil {
		return nil, err
	}
	if err := allowRules.record(settings, allowed); err != nil {
		return nil, err
	}
	skills, err := addClaudeSkills(global)
	return append(append(done, d), skills...), err
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
	if guide(true, true) == "" { // level off: no rule, and none left from an earlier install
		d, err := removeCursorRule()
		return append(done, d...), err
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
	d, err := addGuide(agentsMD, false, true)
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
	d, err := addGuide(agentsMD, true, false)
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

// addClaudeProxy sets ANTHROPIC_BASE_URL to the proxy in Claude's settings.
// Only with an API key or gateway token, or GASCOIGNE_PROXY=on.
func addClaudeProxy(global bool) ([]string, error) {
	key, path, note := claudeProxyVar()
	if key == "" {
		return nil, fmt.Errorf("install -proxy refused: %s", cmp.Or(note, "a base URL is already set (your gateway stays)"))
	}
	settings, err := scoped(global, ".claude/settings.json", ".claude/settings.json")
	if err != nil {
		return nil, err
	}
	d, err := addJSON(settings, removeClaudeProxy, map[string]any{"env": map[string]string{key: proxyURL() + path}},
		key+"="+proxyURL()+path+" (LLM proxy in the background server; the SessionStart hook starts it)", nil)
	if err != nil {
		return nil, err
	}
	if proxyOff() || serverOff() {
		d += "; GASCOIGNE_PROXY or GASCOIGNE_SERVER is off: the server was not started"
	} else if err := serverUp(); err != nil {
		d += "; server not running yet: " + err.Error()
	}
	return []string{d}, nil
}

// removeClaudeProxy drops ANTHROPIC_BASE_URL when it points at the proxy.
func removeClaudeProxy(root *object) []string {
	env, ok := root.get("env").(*object)
	if !ok {
		return nil
	}
	var out []string
	for _, key := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_BEDROCK_BASE_URL"} {
		if u, _ := env.get(key).(string); u == proxyURL() || strings.HasPrefix(u, proxyURL()+"/") {
			env.del(key)
			out = append(out, key+" (LLM proxy)")
		}
	}
	pruneEmpty(root, "env")
	return out
}
