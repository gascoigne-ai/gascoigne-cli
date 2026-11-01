package main

import (
	"strings"
	"testing"
)

// The plugin must register both hooks, and the after hook must send a
// PostToolUse event, or unwrapped commands and MCP results stay long.
func TestOpencodePluginHooks(t *testing.T) {
	got := opencodePlugin("/opt/bin/gascoigne")
	for _, want := range []string{`"tool.execute.before"`, `"tool.execute.after"`, `hook_event_name: "PostToolUse"`, "output.output = JSON.parse(out).output"} {
		if !strings.Contains(got, want) {
			t.Errorf("plugin lacks %s", want)
		}
	}
}
