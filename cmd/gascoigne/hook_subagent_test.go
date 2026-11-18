package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// The agent_id field is taken from Claude Code's documented hook payload
// (session_id, transcript_path, tool_name, tool_input, agent_id, agent_type).
// It is NOT verified against a live subagent capture: no such capture exists.
func TestHookInputAgentID(t *testing.T) {
	tests := []struct {
		name, event, env, want string
	}{
		{"parent has no agent id", `{"session_id":"s1"}`, "", "s1"},
		{"subagent", `{"session_id":"s1","agent_id":"a7","agent_type":"Explore"}`, "", "s1#a7"},
		{"empty agent id", `{"session_id":"s1","agent_id":""}`, "", "s1"},
		{"agent id without session", `{"agent_id":"a7"}`, "", ""},
		{"env override wins", `{"session_id":"s1","agent_id":"a7"}`, "mine", "s1#a7"}, // the hook value stays; execPrefix drops it
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GASCOIGNE_SESSION", tt.env)
			var in hookInput
			if err := json.Unmarshal([]byte(tt.event), &in); err != nil {
				t.Fatal(err)
			}
			if got := hookSession(in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			prefix := execPrefix("gascoigne", "claude", hookSession(in), "", 0)
			if tt.env != "" && strings.Contains(prefix, "-session") {
				t.Fatalf("env session must win: %q", prefix)
			}
		})
	}
}

func TestClaudeHookScopesSessionToAgent(t *testing.T) {
	tests := []struct {
		name, agent, want string
	}{
		{"parent", "", "-session s1 "},
		{"subagent", `,"agent_id":"a7","agent_type":"Explore"`, "-session 's1#a7' "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GASCOIGNE_SESSION", "")
			t.Setenv("GASCOIGNE_AGENT", "")
			event := `{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Bash","tool_input":{"command":"go test ./..."}` + tt.agent + `}`
			out, err := runHook(t, event, "claude")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tt.want) {
				t.Fatalf("want %q in %q", tt.want, out)
			}
		})
	}
}

// rerunOutput is a verbose go test run: the passing noise is shortened, so the
// ledger keeps the output a later run can be folded against.
var rerunOutput = strings.Repeat("=== RUN   TestOk\n--- PASS: TestOk (0.00s)\n", 40) + failureText

const failureText = `--- FAIL: TestParse (0.01s)
    parse_test.go:40: config mismatch
        got:  {Name:api Port:8080 Hosts:[a b c]}
        want: {Name:api Port:9090 Hosts:[a b c]}
    parse_test.go:52: second assertion failed with a long message
FAIL
FAIL	example.com/app/parse	0.012s
FAIL
`

var runs int

// execAs runs the same command through the real exec pipeline and ledger as
// the session a hook would have named.
//
// Each run prints a tag first: the ledger keeps one folded output per distinct
// original, so identical originals would be folded against their own pointers
// (existing behavior, outside this test's concern).
func execAs(t *testing.T, script, session string) string {
	t.Helper()
	runs++
	t.Setenv("RERUN_TAG", fmt.Sprintf("=== RUN   TestTag%d", runs))
	return capture(t, func() {
		// a failing test run named go, so the output is the test family's
		err := cmdExec(t.Context(), []string{"-agent", "claude", "-session", session, "--", script, "test", "-v", "./..."})
		if _, ok := err.(exitCode); err != nil && !ok {
			t.Fatal(err)
		}
	})
}

// A parent and its subagent share the hook's session_id; the agent_id keeps
// what one saw from being pointed at by the other.
func TestRerunPointersStayWithTheAgent(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	file := t.TempDir() + "/go"
	if err := os.WriteFile(file, []byte("#!/bin/sh\nprintf '%s\\n%s' \"$RERUN_TAG\" '"+rerunOutput+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	const ptr = "[↑"
	if out := execAs(t, file, "s1"); strings.Contains(out, ptr) {
		t.Fatalf("first run folded:\n%s", out)
	}
	if out := execAs(t, file, "s1#a7"); strings.Contains(out, ptr) {
		t.Fatalf("subagent got a pointer to the parent's output:\n%s", out)
	}
	if out := execAs(t, file, "s1"); !strings.Contains(out, ptr) {
		t.Fatalf("same agent rerun not folded:\n%s", out)
	}
	if out := execAs(t, file, "s1#a7"); !strings.Contains(out, ptr) {
		t.Fatalf("same subagent rerun not folded:\n%s", out)
	}
	// reverse: a fresh session where the subagent goes first
	if out := execAs(t, file, "s2#b1"); strings.Contains(out, ptr) {
		t.Fatalf("fresh subagent folded:\n%s", out)
	}
	if out := execAs(t, file, "s2"); strings.Contains(out, ptr) {
		t.Fatalf("parent got a pointer to the subagent's output:\n%s", out)
	}
}
