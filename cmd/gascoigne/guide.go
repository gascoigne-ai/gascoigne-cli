package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// guide tells the agent how to get the most out of gascoigne. rewrites:
// the agent's shell commands are rewritten before they run (claude, cursor,
// pi), so how a command is written decides whether it is optimized.
func guide(rewrites bool) string {
	var b strings.Builder
	b.WriteString("gascoigne shrinks shell output and MCP tool results in this session to save context. To get the most from it:\n")
	b.WriteString("- Read and search with shell commands (rg, grep, find, ls, cat, head, tail, git, build and test tools): their output is optimized.\n")
	if rewrites {
		b.WriteString("- Keep each shell call a plain command or a && / ; list, piping only into head or tail. Loops, $( ), redirects, subshells and other pipes run unoptimized.\n")
	}
	b.WriteString("- Read web pages and HTTP APIs with the gascoigne fetch tool (pass query to focus long pages); for large text you already have, use compress.\n")
	b.WriteString("- Ids like §1 in output stand for the path given in the [paths: §1=…] line above them.\n")
	b.WriteString("- Output ending in [gascoigne: … full: gascoigne raw <id>] was shortened. If a detail is missing, run gascoigne raw <id> or call retrieve with the id instead of re-running the command.\n")
	return b.String()
}

// withGuide answers Claude's SessionStart event with the guide; other events
// go to h.
func withGuide(h hookFunc) hookFunc {
	return func(ctx context.Context, c *common, in hookInput, prefix string) (any, error) {
		if in.HookEventName != "SessionStart" {
			return h(ctx, c, in, prefix)
		}
		return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "SessionStart", "additionalContext": guide(true)}}, nil
	}
}

const (
	guideStart = "<!-- gascoigne:start -->"
	guideEnd   = "<!-- gascoigne:end -->"
)

// guideBlock is the guide as a marked block for instruction files (AGENTS.md).
func guideBlock(rewrites bool) string {
	return guideStart + "\n" + guide(rewrites) + guideEnd + "\n"
}

// cutGuide removes the marked block from text.
func cutGuide(text string) (string, bool) {
	start := strings.Index(text, guideStart)
	end := strings.Index(text, guideEnd)
	if start < 0 || end < start {
		return text, false
	}
	end += len(guideEnd)
	if end < len(text) && text[end] == '\n' {
		end++
	}
	before := strings.TrimRight(text[:start], "\n")
	after := text[end:]
	if before != "" && after != "" {
		before += "\n\n"
	} else if before != "" {
		before += "\n"
	}
	return before + strings.TrimLeft(after, "\n"), true
}

// addGuide puts the guide block into an instruction file, replacing an
// earlier one; the rest of the file stays.
func addGuide(file string, rewrites bool) (string, error) {
	backup, err := writeConfig(file, func(text string) (string, error) {
		text, _ = cutGuide(text)
		if text = strings.TrimRight(text, "\n"); text != "" {
			text += "\n\n"
		}
		return text + guideBlock(rewrites), nil
	})
	if err != nil && !errors.Is(err, errUnchanged) {
		return "", err
	}
	return added(file, "usage guide for the agent", backup, err), nil
}

// removeGuide takes the block out of an instruction file, and deletes the
// file when nothing else is left in it.
func removeGuide(file string) ([]string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil
	}
	text, ok := cutGuide(string(data))
	if !ok {
		return nil, nil
	}
	if strings.TrimSpace(text) == "" {
		if err := os.Remove(file); err != nil {
			return nil, err
		}
		return []string{display(file) + ": deleted usage guide (file held nothing else)"}, nil
	}
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		return nil, err
	}
	return []string{display(file) + ": removed usage guide"}, nil
}

const (
	cursorRuleFile   = ".cursor/rules/gascoigne.mdc"
	cursorRuleHeader = "---\ndescription: gascoigne usage\n"
)

// cursorRule is the guide as a Cursor project rule applied to every chat.
func cursorRule() string {
	return cursorRuleHeader + "alwaysApply: true\n---\n" + guideBlock(true)
}

// codexHome is where codex keeps its user config and AGENTS.md.
func codexHome() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return home
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".codex")
}
