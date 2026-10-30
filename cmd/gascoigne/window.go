package main

import (
	"fmt"
	"strconv"
)

// compactWindow is the auto-compact window (tokens) the compaction experiment
// sets for claude: a middle value of the what-ifs in `gascoigne context`.
// Claude Code accepts 100k to 1M (settings key autoCompactWindow).
const compactWindow = 250_000

// syncClaudeWindow makes claude's autoCompactWindow match the compaction
// experiment: set while it is on, gone while it is off. The size gascoigne wrote
// is kept in the prefs, so only that value is ever removed (removeClaudeWindow,
// also run by unwrap), never one the user set. It runs inside syncClaude, after
// removeClaudeSettings has dropped the old value.
func syncClaudeWindow(settings string) (string, error) {
	p := readPrefs()
	if !experimentalOn("compaction") {
		p.Window = 0
		return "", p.save()
	}
	d, err := addJSON(settings, removeClaudeWindow, map[string]any{"autoCompactWindow": compactWindow},
		fmt.Sprintf("autoCompactWindow=%d (experimental: claude compacts when the context reaches it)", compactWindow), nil)
	if err != nil {
		return "", err
	}
	p.Window = compactWindow
	return d, p.save()
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
