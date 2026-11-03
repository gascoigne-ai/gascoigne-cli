package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// guide tells the agent what it cannot guess: what a path id and a [raw:<id>]
// hint stand for, and how to read the original. mcp: the agent has gascoigne's
// MCP tools (pi does not), so the guide names only what the agent can call.
func guide(mcp bool) string {
	if guideLevel() == "off" {
		return ""
	}
	var b strings.Builder
	b.WriteString("gascoigne shrinks shell output and MCP tool results in this session to save context.\n")
	b.WriteString("- Ids like §1 in output stand for the path given in the [paths: §1=…] line above them.\n")
	b.WriteString("- Output ending in [raw:<id>] was shortened. If a detail is missing, run gascoigne raw <id> (add -grep PATTERN, -query WORDS or -lines A-B to read only part)")
	if mcp {
		b.WriteString(" or call retrieve with the id")
	}
	b.WriteString(" instead of re-running the command.\n")
	return b.String()
}

// guideLevels are how much usage text agents get: none, or what an agent
// cannot guess (the hint and path ids). There was a longer "full" level with
// usage tips; it is gone, and a saved "full" now reads as short.
var guideLevels = []string{"off", "short"}

// guideLevel is $GASCOIGNE_GUIDE, else the saved pref, else short. Unknown
// values mean short: a typo must not silently remove the hint explanation.
func guideLevel() string {
	for _, v := range []string{os.Getenv("GASCOIGNE_GUIDE"), readPrefs().Guide} {
		if slices.Contains(guideLevels, v) {
			return v
		}
	}
	return "short"
}

// withGuide answers Claude's SessionStart event with the guide; other events
// go to h.
func withGuide(h hookFunc) hookFunc {
	return func(ctx context.Context, c *common, in hookInput, prefix string) (any, error) {
		if in.HookEventName != "SessionStart" {
			return h(ctx, c, in, prefix)
		}
		ensureProxyFor()
		// Level off keeps only the news of an update.
		text := strings.TrimSpace(in.syncNote + "\n" + guide(true) + resumeContext(in))
		if text == "" {
			return nil, nil
		}
		return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "SessionStart", "additionalContext": text}}, nil
	}
}

const (
	guideStart = "<!-- gascoigne:start -->"
	guideEnd   = "<!-- gascoigne:end -->"
)

// guideBlock is the guide as a marked block for instruction files (AGENTS.md).
// Codex and pi share the project AGENTS.md: the block of the agent installed
// last stays.
func guideBlock(mcp bool) string {
	return guideStart + "\n" + guide(mcp) + guideEnd + "\n"
}

// cutGuide removes the marked block from text.
func cutGuide(text string) (string, bool) { return cutBlock(text, guideStart, guideEnd) }

// cutBlock removes the block between the start and end markers, and the blank
// line it leaves.
func cutBlock(text, startMark, endMark string) (string, bool) {
	start := strings.Index(text, startMark)
	end := strings.Index(text, endMark)
	if start < 0 || end < start {
		return text, false
	}
	end += len(endMark)
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
func addGuide(file string, mcp bool) (string, error) {
	if guide(mcp) == "" { // level off: no block, and none left from an earlier install
		d, err := removeGuide(file)
		return strings.Join(d, "; "), err
	}
	backup, err := writeConfig(file, func(text string) (string, error) {
		text, _ = cutGuide(text)
		if text = strings.TrimRight(text, "\n"); text != "" {
			text += "\n\n"
		}
		return text + guideBlock(mcp), nil
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

// resumeContext is the snapshot a session start after compaction or resume
// carries, when the continuity experiment is on.
func resumeContext(in hookInput) string {
	if in.Source != "compact" && in.Source != "resume" || !experimentalOn("continuity") {
		return ""
	}
	s := resumeSnapshot(in.TranscriptPath)
	if s == "" {
		return ""
	}
	if runs := shortenedRuns(context.Background(), in.SessionID); len(runs) > 0 {
		s += "- shortened outputs you can read in full instead of running the command again (gascoigne raw <id> -grep PATTERN): " + strings.Join(runs, ", ") + "\n"
	}
	return "\n" + s
}
