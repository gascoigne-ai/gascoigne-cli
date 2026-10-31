package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func readEvent(session, path, text string, extra map[string]any) hookInput {
	input := map[string]any{"file_path": path}
	for k, v := range extra {
		input[k] = v
	}
	resp, _ := json.Marshal(map[string]any{"type": "text", "file": map[string]any{"filePath": path, "content": text, "numLines": 20}})
	return hookInput{HookEventName: "PostToolUse", SessionID: session, ToolName: "Read", ToolInput: input, ToolResponse: resp}
}

// folded reports whether the hook answered the Read with the pointer.
func folded(t *testing.T, in hookInput) bool {
	t.Helper()
	var resp any
	json.Unmarshal(in.ToolResponse, &resp)
	out := foldRead(in, resp)
	if out == nil {
		return false
	}
	return out.(map[string]any)["file"].(map[string]any)["content"] == readFoldPointer
}

func TestFoldRead(t *testing.T) {
	text := strings.Repeat("package main // a line of a source file\n", 20)
	tests := []struct {
		name string
		ins  []hookInput // the last one is judged
		want bool
	}{
		{"first read", []hookInput{readEvent("s1", "/p/a.go", text, nil)}, false},
		{"identical re-read", []hookInput{readEvent("s1", "/p/a.go", text, nil), readEvent("s1", "/p/a.go", text, nil)}, true},
		{"file changed", []hookInput{readEvent("s1", "/p/a.go", text, nil), readEvent("s1", "/p/a.go", text+"x\n", nil)}, false},
		{"other path", []hookInput{readEvent("s1", "/p/a.go", text, nil), readEvent("s1", "/p/b.go", text, nil)}, false},
		{"other session", []hookInput{readEvent("s1", "/p/a.go", text, nil), readEvent("s2", "/p/a.go", text, nil)}, false},
		{"other range", []hookInput{readEvent("s1", "/p/a.go", text, nil), readEvent("s1", "/p/a.go", text, map[string]any{"offset": 5.0})}, false},
		{"same range", []hookInput{readEvent("s1", "/p/a.go", text, map[string]any{"offset": 5.0}), readEvent("s1", "/p/a.go", text, map[string]any{"offset": 5.0})}, true},
		{"small file", []hookInput{readEvent("s1", "/p/a.go", "tiny\n", nil), readEvent("s1", "/p/a.go", "tiny\n", nil)}, false},
		{"no session", []hookInput{readEvent("", "/p/a.go", text, nil), readEvent("", "/p/a.go", text, nil)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox(t)
			for _, in := range tt.ins[:len(tt.ins)-1] {
				folded(t, in)
			}
			if got := folded(t, tt.ins[len(tt.ins)-1]); got != tt.want {
				t.Fatalf("folded = %v, want %v", got, tt.want)
			}
		})
	}
}

// A subagent has its own context: the parent's Read is no pointer target for
// it, and its own Reads are not recorded for the parent.
func TestFoldReadSubagent(t *testing.T) {
	sandbox(t)
	text := strings.Repeat("package main // a line of a source file\n", 20)
	sub := readEvent("s1", "/p/a.go", text, nil)
	sub.AgentID = "agent-1"
	folded(t, readEvent("s1", "/p/a.go", text, nil))
	if folded(t, sub) {
		t.Fatal("subagent got a pointer")
	}
	if got := loadReads("s1"); len(got) != 1 {
		t.Fatalf("record = %v", got)
	}
}

// After a compaction, /clear or resume the agent no longer holds the earlier
// text: the next Read must come back whole. Startup keeps the record.
func TestForgetReads(t *testing.T) {
	text := strings.Repeat("package main // a line of a source file\n", 20)
	for _, tt := range []struct {
		source string
		want   bool
	}{{"compact", false}, {"clear", false}, {"resume", false}, {"startup", true}} {
		t.Run(tt.source, func(t *testing.T) {
			sandbox(t)
			in := readEvent("s1", "/p/a.go", text, nil)
			folded(t, in)
			forgetReads(hookInput{HookEventName: "SessionStart", SessionID: "s1", Source: tt.source})
			if got := folded(t, in); got != tt.want {
				t.Fatalf("folded after %s = %v, want %v", tt.source, got, tt.want)
			}
		})
	}
}

// An old record is not trusted: Claude Code may have dropped that result.
func TestFoldReadExpires(t *testing.T) {
	sandbox(t)
	text := strings.Repeat("package main // a line of a source file\n", 20)
	in := readEvent("s1", "/p/a.go", text, nil)
	folded(t, in)
	seen := loadReads("s1")
	for k, v := range seen {
		v.TS = time.Now().Add(-2 * readFoldMaxAge)
		seen[k] = v
	}
	saveReads("s1", seen)
	if folded(t, in) {
		t.Fatal("pointer to an old read")
	}
	if !folded(t, in) {
		t.Fatal("the whole re-read was not recorded")
	}
}

func TestFoldReadOff(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_READ_FOLD", "off")
	text := strings.Repeat("package main // a line of a source file\n", 20)
	in := readEvent("s1", "/p/a.go", text, nil)
	folded(t, in)
	if folded(t, in) {
		t.Fatal("folded while off")
	}
}
