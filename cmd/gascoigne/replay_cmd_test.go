package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// replayTranscript writes a two-turn session with one tool result.
func replayTranscript(t *testing.T) string {
	t.Helper()
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"run it"}}`,
		`{"type":"assistant","message":{"id":"a1","role":"assistant","model":"claude-test","content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}]}}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"file one\nfile two\n"}]}}`,
		`{"type":"assistant","message":{"id":"a2","role":"assistant","model":"claude-test","content":[{"type":"text","text":"done"}]}}`,
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, strings.Join(lines, "\n")+"\n")
	return path
}

func TestCmdReplayReports(t *testing.T) {
	sandbox(t)
	// cmdReplay points the ledger at a throwaway dir for the whole process; have t restore it.
	t.Setenv(ledger.EnvPath, os.Getenv(ledger.EnvPath))
	t.Setenv("GASCOIGNE_SET", "")
	path := replayTranscript(t)

	var err error
	out := capture(t, func() { err = cmdReplay(t.Context(), []string{"-json", path}) })
	if err != nil || !strings.Contains(out, `"sessions": 1`) || !strings.Contains(out, `"Bash"`) {
		t.Fatalf("json: %q, %v", out, err)
	}
	report := filepath.Join(t.TempDir(), "old.json")
	write(t, report, out)

	table := capture(t, func() { err = cmdReplay(t.Context(), []string{path}) })
	if err != nil || table == "" || strings.Contains(table, `"sessions"`) {
		t.Fatalf("table: %q, %v", table, err)
	}
	withCompare := capture(t, func() { err = cmdReplay(t.Context(), []string{"-compare", report, path}) })
	if err != nil || len(withCompare) <= len(table) {
		t.Errorf("compare should print more than the table: %q, %v", withCompare, err)
	}
}

func TestCmdReplayErrors(t *testing.T) {
	sandbox(t)
	t.Setenv(ledger.EnvPath, os.Getenv(ledger.EnvPath))
	t.Setenv("GASCOIGNE_SET", "")
	path := replayTranscript(t)
	bad := filepath.Join(t.TempDir(), "bad.json")
	write(t, bad, "not json")
	for name, args := range map[string][]string{
		"missing compare file": {"-compare", bad + ".none", path},
		"bad compare file":     {"-compare", bad, path},
		"missing transcript":   {path + ".none"},
		"bad override":         {"-set", "nonsense", path},
	} {
		t.Run(name, func(t *testing.T) {
			if err := cmdReplay(t.Context(), args); err == nil {
				t.Error("want an error")
			}
		})
	}
}
