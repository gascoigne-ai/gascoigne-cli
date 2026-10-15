package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Resume snapshot (experimental: continuity). After an agent compacts its
// conversation it forgets which files it edited and what it was doing; the
// transcript still has it. A SessionStart hook with source compact or resume
// adds a short, capped summary built from the transcript: what the user asked
// last, open tasks, files edited, recent failed commands. Tool output is never
// used, only the agent's own calls, and secrets in command lines are cut.

const (
	snapshotMaxPrompt   = 300
	snapshotMaxFiles    = 15
	snapshotMaxTasks    = 8
	snapshotMaxFailures = 3
	snapshotMaxCommand  = 100
	snapshotMaxRuns     = 6
	// snapshotMinSaved is how many tokens a shortened output must have saved
	// to be worth naming: smaller ones are cheaper to run again.
	snapshotMinSaved = 200
	// transcriptScan caps how much of a transcript is read.
	transcriptScan = 16 << 20
)

// secretAssign matches NAME=value pairs whose name suggests a credential, and
// bearer tokens, in a command line.
var secretAssign = lazyre.New(`(?i)\b([a-z0-9_]*(?:key|token|secret|password|passwd|credential)[a-z0-9_]*=)\S+|(bearer\s+)\S+`)

// snapshotLine is the part of a Claude Code transcript line the snapshot uses.
type snapshotLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"` // a subagent's own turns, not the user's conversation
	Message     struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type snapshotBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	ID        string          `json:"id"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
}

// resumeSnapshot summarizes the transcript at path; "" when there is nothing
// worth saying or the file cannot be read.
func resumeSnapshot(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var (
		prompt   string
		files    []string
		todos    []string
		commands = map[string]string{} // Bash tool_use id → command
		failed   []string
	)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), transcriptScan)
	for sc.Scan() {
		var l snapshotLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.IsSidechain {
			continue
		}
		for _, b := range contentBlocks(l.Message.Content) {
			switch {
			case l.Type == "user" && b.Type == "text" && isPrompt(b.Text):
				prompt = b.Text
			case l.Type == "user" && b.Type == "tool_result" && b.IsError:
				if c, ok := commands[b.ToolUseID]; ok {
					failed = append(slices.DeleteFunc(failed, func(x string) bool { return x == c }), c)
				}
			case l.Type == "assistant" && b.Type == "tool_use":
				switch b.Name {
				case "Edit", "Write", "MultiEdit", "NotebookEdit":
					if p := editedPath(b.Input); p != "" {
						files = append(slices.DeleteFunc(files, func(x string) bool { return x == p }), p)
					}
				case "Bash":
					commands[b.ID] = commandOf(b.Input)
				case "TodoWrite":
					todos = openTodos(b.Input)
				}
			}
		}
	}
	return formatSnapshot(prompt, todos, files, failed)
}

// contentBlocks reads a message's content, which is a string or a list of blocks.
func contentBlocks(raw json.RawMessage) []snapshotBlock {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []snapshotBlock{{Type: "text", Text: s}}
	}
	var bs []snapshotBlock
	json.Unmarshal(raw, &bs)
	return bs
}

// isPrompt tells what the user typed from what the harness injected
// (reminders, command wrappers, the compaction summary).
func isPrompt(text string) bool {
	t := strings.TrimSpace(text)
	return t != "" && !strings.HasPrefix(t, "<") && !strings.HasPrefix(t, "This session is being continued") && !strings.HasPrefix(t, "[Request interrupted")
}

func editedPath(input json.RawMessage) string {
	var in struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
	}
	json.Unmarshal(input, &in)
	return in.FilePath + in.NotebookPath
}

func commandOf(input json.RawMessage) string {
	var in struct {
		Command string `json:"command"`
	}
	json.Unmarshal(input, &in)
	return oneLine(secretAssign.Regexp().ReplaceAllString(in.Command, "${1}${2}…"), snapshotMaxCommand)
}

// openTodos are the unfinished items of the latest TodoWrite call.
func openTodos(input json.RawMessage) []string {
	var in struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	json.Unmarshal(input, &in)
	var out []string
	for _, t := range in.Todos {
		if t.Status != "completed" {
			out = append(out, fmt.Sprintf("[%s] %s", t.Status, oneLine(t.Content, snapshotMaxPrompt)))
		}
	}
	return out
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// shortenedRuns names the outputs gascoigne shortened in this session and that
// can still be read in full, so that after a compaction the agent reads them
// (gascoigne raw <id> -grep PATTERN) instead of running the command again.
// Only the command and the id are used, never the output. The ledger is the
// default one: a custom path in the config is not followed.
func shortenedRuns(ctx context.Context, session string) []string {
	if session == "" {
		return nil
	}
	l := ledger.New(&ledger.Config{})
	if l.Start(ctx, nil) != nil {
		return nil
	}
	defer l.Shutdown(ctx)
	runs, err := l.RecentRuns(ctx, session, snapshotMaxRuns*3)
	if err != nil {
		return nil
	}
	var out []string
	for _, r := range runs {
		if r.RawTokens-r.OutTokens < snapshotMinSaved {
			continue
		}
		key := r.Key
		if i := strings.Index(key, ":"); i >= 0 && i < 6 {
			key = key[i+1:] // cmd:, file:, url:, tool:
		}
		key = oneLine(secretAssign.Regexp().ReplaceAllString(key, "${1}${2}…"), snapshotMaxCommand)
		out = append(out, fmt.Sprintf("`%s` → %s", key, r.RawID))
		if len(out) == snapshotMaxRuns {
			break
		}
	}
	return out
}

func formatSnapshot(prompt string, todos, files, failed []string) string {
	if prompt == "" && len(todos) == 0 && len(files) == 0 && len(failed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("gascoigne: your state before the conversation was compacted or resumed, from the transcript:\n")
	if prompt != "" {
		fmt.Fprintf(&b, "- last request: %s\n", oneLine(prompt, snapshotMaxPrompt))
	}
	if len(todos) > 0 {
		b.WriteString("- open tasks:\n")
		for _, t := range todos[:min(len(todos), snapshotMaxTasks)] {
			fmt.Fprintf(&b, "  - %s\n", t)
		}
	}
	if len(files) > 0 {
		recent := files[max(0, len(files)-snapshotMaxFiles):]
		slices.Reverse(recent) // newest first
		more := ""
		if n := len(files) - len(recent); n > 0 {
			more = fmt.Sprintf(" (+%d older)", n)
		}
		fmt.Fprintf(&b, "- files edited, newest first: %s%s\n", strings.Join(recent, ", "), more)
	}
	if len(failed) > 0 {
		recent := failed[max(0, len(failed)-snapshotMaxFailures):]
		slices.Reverse(recent)
		fmt.Fprintf(&b, "- recent failed commands: `%s`\n", strings.Join(recent, "`, `"))
	}
	return b.String()
}
