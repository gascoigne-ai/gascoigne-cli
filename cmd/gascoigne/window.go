package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Claude Code accepts an auto-compact window from 100k to 1M tokens
// (settings key autoCompactWindow); install -window writes it.
const (
	minWindow = 100_000
	maxWindow = 1_000_000
)

// parseWindow reads a window size: 300000, 300k or 1M. "auto" and "off" give 0,
// which removes what gascoigne wrote so the agent's own default applies again.
func parseWindow(s string) (int, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "auto" || s == "off" {
		return 0, nil
	}
	mult := 1
	switch {
	case strings.HasSuffix(s, "k"):
		mult, s = 1_000, strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "m"):
		mult, s = 1_000_000, strings.TrimSuffix(s, "m")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 || n*mult < minWindow || n*mult > maxWindow {
		return 0, fmt.Errorf("install: -window wants auto or a size from 100k to 1M tokens (300k, 1M, 300000)")
	}
	return n * mult, nil
}

// addClaudeWindow sets claude's auto-compact window for good, or with 0 removes
// the one gascoigne set. The size is kept in the prefs so that unwrap and a later
// -window auto take out only what gascoigne wrote, never the user's own value.
func addClaudeWindow(global bool, tokens int) ([]string, error) {
	settings, err := scoped(global, ".claude/settings.json", ".claude/settings.json")
	if err != nil {
		return nil, err
	}
	var done []string
	if tokens == 0 {
		done, err = editJSON(settings, removeClaudeWindow)
	} else {
		var d string
		d, err = addJSON(settings, removeClaudeWindow, map[string]any{"autoCompactWindow": tokens},
			fmt.Sprintf("autoCompactWindow=%d (claude compacts when the context reaches it)", tokens), nil)
		done = []string{d}
	}
	if err != nil {
		return done, err
	}
	p := readPrefs()
	p.Window = tokens
	return done, p.save()
}

// removeClaudeWindow drops autoCompactWindow when it is the size gascoigne wrote.
func removeClaudeWindow(root *object) []string {
	n := readPrefs().Window
	if n == 0 || fmt.Sprint(root.get("autoCompactWindow")) != strconv.Itoa(n) {
		return nil
	}
	root.del("autoCompactWindow")
	return []string{"autoCompactWindow"}
}
