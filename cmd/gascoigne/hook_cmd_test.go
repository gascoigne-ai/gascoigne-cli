package main

import (
	"os"
	"strings"
	"testing"
)

// runHook feeds event to cmdHook as stdin and returns what it printed.
func runHook(t *testing.T, event string, args ...string) (string, error) {
	t.Helper()
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString(event); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	old := os.Stdin
	os.Stdin = reader
	defer func() { os.Stdin = old }()
	var runErr error
	out := capture(t, func() { runErr = cmdHook(t.Context(), args) })
	return out, runErr
}

func TestCmdHookUnknownAgent(t *testing.T) {
	if _, err := runHook(t, "{}", "nope"); err == nil || !strings.Contains(err.Error(), "want an agent") {
		t.Errorf("err %v", err)
	}
}

func TestCmdHookUnreadableEventIsNotAnError(t *testing.T) {
	out, err := runHook(t, "not json", "claude")
	if err != nil || out != "" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestCmdHookShellCallIsRewritten(t *testing.T) {
	out, err := runHook(t, `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "updatedInput") || !strings.Contains(out, "go test ./...") {
		t.Errorf("reply %q", out)
	}
}

func TestCmdHookPromptWithoutNoteStaysSilent(t *testing.T) {
	out, err := runHook(t, `{"hook_event_name":"UserPromptSubmit","session_id":"s1"}`, "claude")
	if err != nil || out != "" {
		t.Errorf("out %q err %v", out, err)
	}
}

func TestCmdHookManualModeSkipsToolEvents(t *testing.T) {
	t.Setenv("GASCOIGNE_MCP_MODE", "manual")
	out, err := runHook(t, `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`, "claude")
	if err != nil || out != "" {
		t.Errorf("out %q err %v", out, err)
	}
}
