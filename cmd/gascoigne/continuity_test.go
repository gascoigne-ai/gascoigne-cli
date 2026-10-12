package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// transcript is a Claude Code transcript in the shape the snapshot reads.
const transcript = `{"type":"user","message":{"role":"user","content":"add a settings page"}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/p/a.go"}},{"type":"tool_use","id":"t2","name":"Write","input":{"file_path":"/p/b.go"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"TOP-SECRET-OUTPUT sk-live-123"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t3","name":"Bash","input":{"command":"API_TOKEN=abc123 go test ./... --bearer x"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t3","is_error":true,"content":"FAIL: leaked-output"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t4","name":"TodoWrite","input":{"todos":[{"content":"write tests","status":"in_progress"},{"content":"done thing","status":"completed"},{"content":"update docs","status":"pending"}]}}]}}
{"type":"user","isSidechain":true,"message":{"content":"a subagent's prompt"}}
{"type":"user","message":{"content":[{"type":"text","text":"<system-reminder>ignore me</system-reminder>"}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t5","name":"Edit","input":{"file_path":"/p/a.go"}}]}}
{"type":"user","message":{"role":"user","content":"now make it dark mode too"}}
not json
`

func writeTranscript(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResumeSnapshot(t *testing.T) {
	got := resumeSnapshot(writeTranscript(t, transcript))
	for _, want := range []string{
		"last request: now make it dark mode too",
		"[in_progress] write tests",
		"[pending] update docs",
		"files edited, newest first: /p/a.go, /p/b.go", // a.go edited again after b.go
		"recent failed commands: `API_TOKEN=… go test ./... --bearer …`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, got)
		}
	}
	for _, leak := range []string{"TOP-SECRET-OUTPUT", "sk-live-123", "abc123", "leaked-output", "subagent", "ignore me", "done thing"} {
		if strings.Contains(got, leak) {
			t.Errorf("snapshot leaks %q:\n%s", leak, got)
		}
	}
	if again := resumeSnapshot(writeTranscript(t, transcript)); again != got {
		t.Error("the snapshot must be deterministic")
	}
}

func TestResumeSnapshotIsCapped(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"type":"user","message":{"content":"` + strings.Repeat("long ", 400) + `"}}` + "\n")
	for i := range 200 {
		fmt.Fprintf(&b, `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"e%d","name":"Edit","input":{"file_path":"/p/file%d.go"}},{"type":"tool_use","id":"b%d","name":"Bash","input":{"command":"cmd%d %s"}}]}}`+"\n", i, i, i, i, strings.Repeat("x", 300))
		fmt.Fprintf(&b, `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"b%d","is_error":true,"content":"no"}]}}`+"\n", i)
	}
	got := resumeSnapshot(writeTranscript(t, b.String()))
	if len(got) > 2500 {
		t.Errorf("snapshot is %d bytes, want a few hundred tokens:\n%s", len(got), got)
	}
	if !strings.Contains(got, "/p/file199.go") || strings.Contains(got, "/p/file100.go") || !strings.Contains(got, "(+185 older)") {
		t.Errorf("keeps the newest files only:\n%s", got)
	}
	if strings.Count(got, "cmd") != 3 {
		t.Errorf("three failed commands at most:\n%s", got)
	}
}

func TestResumeSnapshotEmpty(t *testing.T) {
	for name, body := range map[string]string{
		"no file":    "",
		"only noise": `{"type":"user","message":{"content":"<command-name>/clear</command-name>"}}` + "\n",
	} {
		path := filepath.Join(t.TempDir(), "missing")
		if body != "" {
			path = writeTranscript(t, body)
		}
		if got := resumeSnapshot(path); got != "" {
			t.Errorf("%s: %q", name, got)
		}
	}
}

func TestSessionStartAddsSnapshotOnlyWhenEnabled(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_GUIDE", "off")
	path := writeTranscript(t, transcript)
	h := withGuide(func(context.Context, *common, hookInput, string) (any, error) { return nil, nil })
	ctxOf := func(source string) string {
		reply, err := h(t.Context(), &common{}, hookInput{HookEventName: "SessionStart", Source: source, TranscriptPath: path}, "")
		if err != nil || reply == nil {
			return ""
		}
		return reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	}
	if ctxOf("compact") != "" {
		t.Fatal("off by default: it is experimental")
	}
	t.Setenv("GASCOIGNE_EXPERIMENTAL", "continuity")
	if got := ctxOf("compact"); !strings.Contains(got, "last request: now make it dark mode too") {
		t.Errorf("after compaction:\n%s", got)
	}
	if got := ctxOf("resume"); !strings.Contains(got, "files edited") {
		t.Errorf("on resume:\n%s", got)
	}
	if ctxOf("startup") != "" || ctxOf("clear") != "" {
		t.Error("a fresh session or /clear starts clean")
	}
}

// After a compaction the agent forgets which outputs gascoigne shortened. The
// snapshot names them with their raw ids, from the ledger and never from output.
func TestSnapshotNamesShortenedRuns(t *testing.T) {
	sandbox(t)
	db := filepath.Join(t.TempDir(), "l.db")
	t.Setenv("GASCOIGNE_LEDGER", db)
	t.Setenv("GASCOIGNE_GUIDE", "off")
	t.Setenv("GASCOIGNE_EXPERIMENTAL", "continuity")
	l := ledger.New(&ledger.Config{Path: db})
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	rec := func(session, key, raw string, in, out int) {
		t.Helper()
		err := l.Record(t.Context(), component.Event{TS: time.Now(), Session: session, Agent: "claude", Key: key, Route: "default",
			Command: "x", RawTokens: in, OutTokens: out, RawID: raw, Outcome: component.Optimized})
		if err != nil {
			t.Fatal(err)
		}
	}
	rec("s1", "cmd:go test ./...", "aaaa1111", 5000, 400)
	rec("s1", "cmd:go test ./...", "bbbb2222", 5200, 420) // the same command again: the latest id
	rec("s1", "cmd:API_TOKEN=abc123 make build", "cccc3333", 3000, 300)
	rec("s1", "cmd:ls", "dddd4444", 300, 200)       // saved too little to name
	rec("s2", "cmd:git log", "eeee5555", 9000, 900) // another session
	l.Shutdown(t.Context())

	h := withGuide(func(context.Context, *common, hookInput, string) (any, error) { return nil, nil })
	reply, err := h(t.Context(), &common{}, hookInput{HookEventName: "SessionStart", Source: "compact", SessionID: "s1", TranscriptPath: writeTranscript(t, transcript)}, "")
	if err != nil || reply == nil {
		t.Fatalf("%v %v", reply, err)
	}
	got := reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	for _, want := range []string{"shortened outputs you can read in full", "`go test ./...` → bbbb2222", "`API_TOKEN=… make build` → cccc3333"} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
	for _, leak := range []string{"aaaa1111", "dddd4444", "eeee5555", "abc123"} {
		if strings.Contains(got, leak) {
			t.Errorf("names %q:\n%s", leak, got)
		}
	}
	if strings.Count(got, "→") != 2 {
		t.Errorf("one entry per command:\n%s", got)
	}
}
