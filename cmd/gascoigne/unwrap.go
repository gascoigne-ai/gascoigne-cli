package main

import (
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

// cmdUnwrap removes what registers gascoigne with agents for good (the config
// wrap -print shows), from user and project config files. Edited files keep
// their original as <file>.gascoigne-bak.
func cmdUnwrap(_ context.Context, args []string) error {
	fset := newFlags("unwrap")
	if err := fset.Parse(args); err != nil {
		return err
	}
	names := fset.Args()
	if len(names) == 0 {
		names = strings.Split(agentNames(), ", ")
	}
	var errs []error
	changed := false
	for _, name := range names {
		a, ok := agents[name]
		if !ok {
			return unknownAgent(name)
		}
		done, err := a.remove()
		errs = append(errs, err)
		for _, d := range done {
			fmt.Printf("%s: %s\n", name, d)
			changed = true
		}
	}
	err := errors.Join(errs...)
	if !changed && err == nil {
		fmt.Printf("gascoigne is not registered with %s\n", strings.Join(names, ", "))
	}
	return err
}

// configFiles are the user (home-relative) and project (cwd-relative) paths of
// one agent config file; a project in the home dir lists each file once.
func configFiles(user string, project ...string) []string {
	var files []string
	if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, user))
	}
	for _, f := range project {
		abs, err := filepath.Abs(f)
		if err != nil || !slices.Contains(files, abs) {
			files = append(files, f)
		}
	}
	return files
}

func removeClaude() ([]string, error) {
	var done []string
	var errs []error
	for _, f := range configFiles(".claude/settings.json", ".claude/settings.json", ".claude/settings.local.json") {
		d, err := editJSON(f, func(root *object) []string {
			return append(removeClaudeSettings(root), removeDenies(f, root)...)
		})
		done, errs = append(done, d...), append(errs, err)
	}
	d, err := editJSON(".mcp.json", mcpEdit)
	done, errs = append(done, d...), append(errs, err)
	d, err = editJSON(claudeJSON(), unproxyClaudeState)
	done, errs = append(done, d...), append(errs, err)
	// User and local MCP servers live in ~/.claude.json, which Claude Code
	// itself keeps writing: let its CLI remove them.
	if _, err := exec.LookPath("claude"); err == nil {
		for _, scope := range []string{"user", "local"} {
			if exec.Command("claude", "mcp", "remove", "--scope", scope, "gascoigne").Run() == nil {
				done = append(done, "claude mcp: removed MCP server gascoigne ("+scope+" scope)")
			}
		}
	}
	return done, errors.Join(errs...)
}

// removeClaudeSettings drops the hook and env gascoigne adds to Claude settings.
func removeClaudeSettings(root *object) []string {
	var what []string
	if n := removeHooks(root, "PreToolUse", ourHook("claude")); n > 0 {
		what = append(what, "PreToolUse hook")
	}
	if n := removeHooks(root, "SessionStart", ourHook("claude")); n > 0 {
		what = append(what, "SessionStart hook")
	}
	if n := removeHooks(root, "PostToolUse", ourHook("claude")); n > 0 {
		what = append(what, "PostToolUse hook")
	}
	if n := removeHooks(root, "UserPromptSubmit", ourHook("claude")); n > 0 {
		what = append(what, "UserPromptSubmit hook")
	}
	what = append(what, removeClaudeProxy(root)...)
	if env, ok := root.get("env").(*object); ok && env.get("GASCOIGNE_AGENT") == "claude" {
		env.del("GASCOIGNE_AGENT")
		pruneEmpty(root, "env")
		what = append(what, "GASCOIGNE_AGENT env")
	}
	return what
}

func removeCursorHooks(root *object) []string {
	if removeHooks(root, "preToolUse", ourHook("cursor")) > 0 {
		return []string{"preToolUse hook"}
	}
	return nil
}

func removeCursor() ([]string, error) {
	var done []string
	var errs []error
	for _, f := range configFiles(".cursor/hooks.json", ".cursor/hooks.json") {
		d, err := editJSON(f, removeCursorHooks)
		done, errs = append(done, d...), append(errs, err)
	}
	for _, f := range configFiles(".cursor/mcp.json", ".cursor/mcp.json") {
		d, err := editJSON(f, mcpEdit)
		done, errs = append(done, d...), append(errs, err)
	}
	if data, err := os.ReadFile(cursorRuleFile); err == nil && string(data) == cursorRule() || err == nil && strings.HasPrefix(string(data), cursorRuleHeader) {
		if err := os.Remove(cursorRuleFile); err != nil {
			errs = append(errs, err)
		} else {
			done = append(done, cursorRuleFile+": deleted rule")
		}
	}
	return done, errors.Join(errs...)
}

func mcpEdit(root *object) []string {
	var what []string
	if names := unproxyServers(obj(root.get("mcpServers"))); len(names) > 0 {
		what = append(what, "proxy from MCP servers "+strings.Join(names, ", "))
	}
	if removeMCP(root) {
		what = append(what, "MCP server gascoigne")
	}
	return what
}

// unproxyClaudeState restores proxied servers in ~/.claude.json: the user's
// and every project's local ones.
func unproxyClaudeState(root *object) []string {
	names := unproxyServers(obj(root.get("mcpServers")))
	if projects := obj(root.get("projects")); projects != nil {
		for _, p := range projects.keys {
			names = append(names, unproxyServers(obj(path(projects, p, "mcpServers")))...)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return []string{"proxy from MCP servers " + strings.Join(names, ", ")}
}

func removeCodex() ([]string, error) {
	user := ".codex/config.toml"
	files := configFiles(user, ".codex/config.toml")
	if home := os.Getenv("CODEX_HOME"); home != "" {
		files[0] = filepath.Join(home, "config.toml")
	}
	var done []string
	var errs []error
	for _, f := range files {
		d, err := editFile(f, removeCodexTOML)
		done, errs = append(done, d...), append(errs, err)
	}
	for _, f := range []string{filepath.Join(codexHome(), "AGENTS.md"), "AGENTS.md"} {
		d, err := removeGuide(f)
		done, errs = append(done, d...), append(errs, err)
	}
	return done, errors.Join(errs...)
}

const piHeader = "// gascoigne: routes bash tool calls"

func removePi() ([]string, error) {
	files := configFiles(".pi/agent/extensions/gascoigne.ts", ".pi/extensions/gascoigne.ts")
	if dir, err := os.UserCacheDir(); err == nil {
		files = append(files, filepath.Join(dir, "gascoigne", "pi", "gascoigne.ts")) // left by wrap pi
	}
	var done []string
	var errs []error
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || !strings.HasPrefix(string(data), piHeader) {
			continue // missing, or not the extension gascoigne wrote
		}
		if err := os.Remove(f); err != nil {
			errs = append(errs, err)
			continue
		}
		done = append(done, display(f)+": deleted extension")
	}
	for _, f := range configFiles(".pi/agent/AGENTS.md", "AGENTS.md") {
		d, err := removeGuide(f)
		done, errs = append(done, d...), append(errs, err)
	}
	return done, errors.Join(errs...)
}

// editJSON applies edit to a JSON config file and saves it when edit names
// what it removed. Missing files are skipped; unparsable ones mentioning
// gascoigne are reported, never rewritten.
func editJSON(file string, edit func(root *object) []string) ([]string, error) {
	var parseErr error
	done, err := editFile(file, func(text string) (string, []string) {
		v, err := parseJSON([]byte(text))
		root, ok := v.(*object)
		if err != nil || !ok {
			if strings.Contains(text, "gascoigne") {
				parseErr = fmt.Errorf("%s: not a JSON object, edit it by hand: %v", display(file), err)
			}
			return text, nil
		}
		what := edit(root)
		if len(what) == 0 {
			return text, nil
		}
		out, err := formatJSON(root)
		if err != nil {
			parseErr = fmt.Errorf("%s: %w", display(file), err)
			return text, nil
		}
		return string(out), what
	})
	return done, errors.Join(err, parseErr)
}

// editFile rewrites file with edit, keeping the original next to it.
func editFile(file string, edit func(text string) (string, []string)) ([]string, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out, what := edit(string(data))
	if len(what) == 0 {
		return nil, nil
	}
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	backup := file + ".gascoigne-bak"
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("back up %s: %w", file, err)
	}
	if err := os.WriteFile(file, []byte(out), info.Mode().Perm()); err != nil {
		return nil, fmt.Errorf("write %s: %w", file, err)
	}
	return []string{fmt.Sprintf("%s: removed %s (backup: %s)", display(file), strings.Join(what, ", "), filepath.Base(backup))}, nil
}

// display shortens home paths to ~/….
func display(file string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if rel, ok := strings.CutPrefix(file, home+string(filepath.Separator)); ok {
			return "~/" + rel
		}
	}
	return file
}

// removeCodexTOML undoes install in a codex config: proxied servers get
// their own command back, gascoigne's tables go.
func removeCodexTOML(text string) (string, []string) {
	text, proxied := unproxyCodexTOML(text)
	text, what := removeCodexEntries(text)
	if len(proxied) > 0 {
		what = append(what, "proxy from MCP servers "+strings.Join(proxied, ", "))
	}
	return text, what
}

// removeCodexEntries drops [mcp_servers.gascoigne] tables and the
// PostToolUse hooks running gascoigne hook codex, as wrap -print codex writes
// them. A hook group goes whole only when all its handlers are ours.
func removeCodexEntries(text string) (string, []string) {
	sections := tomlSections(text)
	var what []string
	drop := make([]bool, len(sections))
	for i, s := range sections {
		name := strings.Trim(s.header, "[] ")
		switch {
		case name == "mcp_servers.gascoigne" || strings.HasPrefix(name, "mcp_servers.gascoigne."):
			drop[i] = true
			if !slices.Contains(what, "MCP server gascoigne") {
				what = append(what, "MCP server gascoigne")
			}
		case strings.HasPrefix(s.header, "[[hooks.") && !strings.HasSuffix(s.header, ".hooks]]"):
			event := strings.TrimSuffix(strings.TrimPrefix(s.header, "[[hooks."), "]]")
			var handlers, ours []int
			for j := i + 1; j < len(sections) && sections[j].header == "[[hooks."+event+".hooks]]"; j++ {
				handlers = append(handlers, j)
				if strings.Contains(sections[j].body, "gascoigne") && strings.Contains(sections[j].body, ` hook codex"`) {
					ours = append(ours, j)
				}
			}
			if len(ours) == 0 {
				continue
			}
			for _, j := range ours {
				drop[j] = true
			}
			drop[i] = len(ours) == len(handlers)
			what = append(what, event+" hook")
		}
	}
	if len(what) == 0 {
		return text, nil
	}
	var b strings.Builder
	for i, s := range sections {
		if !drop[i] {
			b.WriteString(s.header + s.body)
		}
	}
	return b.String(), what
}

type tomlSection struct {
	header string // "[table]" line without newline, "" for the preamble
	body   string // everything up to the next header, newlines included
}

// tomlSections splits TOML text at table headers.
func tomlSections(text string) []tomlSection {
	sections := []tomlSection{{}}
	for _, l := range strings.SplitAfter(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			sections = append(sections, tomlSection{header: strings.TrimRight(l, "\n"), body: "\n"})
			continue
		}
		sections[len(sections)-1].body += l
	}
	if !strings.HasSuffix(text, "\n") { // last header line had no newline
		last := &sections[len(sections)-1]
		last.body = strings.TrimSuffix(last.body, "\n")
	}
	return sections
}
