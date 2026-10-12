package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// reviewFixture is two weeks of a realistic setup, today being 2026-10-14:
//   - github: 30 tools, 6k tokens, used every few days
//   - linear: 12 tools, 4k tokens, seen for 12 days, never called
//   - sentry: 5 tools, 2.4k tokens, one call in two weeks
//   - figma: 8 tools, seen since yesterday, never called yet
//   - db: stays direct, called through the hook only
func reviewFixture() ([]ledger.ToolStat, []ledger.CommandDay, time.Time) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	day := func(n int) string { return now.AddDate(0, 0, -n).Format("2006-01-02") }
	var stats []ledger.ToolStat
	list := func(server string, tools, tokens int, from int) {
		for d := from; d >= 0; d-- {
			for i := range tools {
				stats = append(stats, ledger.ToolStat{Day: day(d), Agent: "claude", Server: server, Tool: fmt.Sprintf("tool_%02d", i), Tokens: tokens / tools})
			}
		}
	}
	list("github", 30, 6000, 13)
	list("linear", 12, 4000, 11)
	list("sentry", 5, 2400, 13)
	list("figma", 8, 1600, 1)
	call := func(server, tool string, ago, n int) {
		stats = append(stats, ledger.ToolStat{Day: day(ago), Agent: "claude", Server: server, Tool: tool, Calls: n})
	}
	call("github", "tool_00", 0, 5)
	call("github", "tool_00", 3, 4)
	call("github", "tool_07", 9, 2) // the week before
	call("sentry", "tool_01", 10, 1)
	cmds := []ledger.CommandDay{
		{Day: day(1), Agent: "claude", Command: "mcp__db__query", Runs: 7},
		{Day: day(8), Agent: "claude", Command: "mcp__db__query", Runs: 3},
		{Day: day(1), Agent: "claude", Command: "mcp__github__tool_00", Runs: 4}, // the hook saw proxied calls too
		{Day: day(1), Agent: "claude", Command: "mcp__gascoigne__fetch", Runs: 9},
	}
	return stats, cmds, now
}

func TestBuildReview(t *testing.T) {
	stats, cmds, now := reviewFixture()
	r := buildReview(stats, cmds, now)
	got := map[string]mcpServer{}
	for _, s := range r.Servers {
		got[s.Name] = s
	}
	tests := []struct {
		server  string
		verdict string
		tokens  int
		idle    int
		calls7  int
		prev    int
		sized   bool
	}{
		{"github", verdictUsed, 6000, 5600, 9, 2, true}, // the hook's 4 calls of tool_00 are not counted twice
		{"linear", verdictUnused, 3996, 3996, 0, 0, true},
		{"sentry", verdictRarely, 2400, 1920, 0, 1, true},
		{"figma", verdictLearning, 1600, 1600, 0, 0, true},
		{"db", verdictUsed, 0, 0, 7, 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.server, func(t *testing.T) {
			s, ok := got[tt.server]
			if !ok {
				t.Fatalf("server missing from %v", r.Servers)
			}
			if s.Verdict != tt.verdict || s.Tokens != tt.tokens || s.IdleToks != tt.idle ||
				s.Calls7 != tt.calls7 || s.CallsPrev != tt.prev || s.Sized != tt.sized {
				t.Fatalf("got %+v", s)
			}
		})
	}
	if _, ok := got["gascoigne"]; ok {
		t.Fatal("gascoigne's own server is reviewed")
	}
	if r.Days != 14 {
		t.Fatalf("days = %d", r.Days)
	}
	if r.Servers[0].Name != "github" {
		t.Fatalf("order: biggest first, got %s", r.Servers[0].Name)
	}
}

func TestBuildReviewYoungLedger(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	stats := []ledger.ToolStat{{Day: "2026-10-13", Agent: "claude", Server: "linear", Tool: "a", Tokens: 3000}}
	r := buildReview(stats, nil, now)
	if len(r.Servers) != 1 || r.Servers[0].Verdict != verdictLearning {
		t.Fatalf("got %+v", r.Servers)
	}
	if len(r.Notes) == 0 || !strings.Contains(r.Notes[0], "day(s) of data") {
		t.Fatalf("notes %v", r.Notes)
	}
}

func TestBuildReviewEmpty(t *testing.T) {
	r := buildReview(nil, nil, time.Now())
	if len(r.Servers) != 0 || len(r.Notes) != 1 {
		t.Fatalf("got %+v", r)
	}
	var b bytes.Buffer
	printReview(&b, r)
	if !strings.Contains(b.String(), "mcp-proxy") {
		t.Fatalf("empty text %q", b.String())
	}
}

func TestPrintReview(t *testing.T) {
	stats, cmds, now := reviewFixture()
	var b bytes.Buffer
	printReview(&b, buildReview(stats, cmds, now))
	out := b.String()
	for _, want := range []string{"SERVER", "github", "used", "linear", "unused", "db", "?", "stay direct"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestSplitMCPName(t *testing.T) {
	for name, want := range map[string]string{
		"mcp__db__query":            "db/query",
		"mcp__claude_ai_Gmail__x_y": "claude_ai_Gmail/x_y",
		"mcp__gascoigne__fetch":     "",
		"mcp__db":                   "",
		"mcp__db__call_tool":        "", // the proxy's lazy tools are not servers' tools
		"mcp__db__search_tools":     "",
		"query":                     "",
	} {
		server, tool, ok := splitMCPName(name)
		if got := server + "/" + tool; ok != (want != "") || (ok && got != want) {
			t.Errorf("%s: %q %v, want %q", name, got, ok, want)
		}
	}
}

// A direct server that shares common tool names with a proxied one keeps its calls.
func TestBuildReviewCommonToolNames(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	today := now.Format("2006-01-02")
	stats := []ledger.ToolStat{
		{Day: today, Agent: "claude", Server: "github#aaaa1111", Tool: "search", Tokens: 300, Calls: 4},
		{Day: today, Agent: "claude", Server: "github#aaaa1111", Tool: "create", Tokens: 500},
	}
	cmds := []ledger.CommandDay{
		{Day: today, Agent: "claude", Command: "mcp__github__search", Runs: 4}, // the hook saw the proxied call too
		{Day: today, Agent: "claude", Command: "mcp__db__search", Runs: 3},
		{Day: today, Agent: "claude", Command: "mcp__db__query", Runs: 2},
	}
	got := map[string]mcpServer{}
	for _, s := range buildReview(stats, cmds, now).Servers {
		got[s.ID] = s
	}
	if len(got) != 2 {
		t.Fatalf("servers %v", got)
	}
	if s := got["github#aaaa1111"]; s.Calls7 != 4 {
		t.Fatalf("github counted %d calls, want the proxy's 4 once", s.Calls7)
	}
	if s := got["db"]; s.Calls7 != 5 || s.Sized {
		t.Fatalf("db %+v: its search and query calls are its own", s)
	}
}

// Two configurations of one server binary stay apart, and say how.
func TestBuildReviewDuplicateNames(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	today := now.Format("2006-01-02")
	stats := []ledger.ToolStat{
		{Day: today, Agent: "claude", Server: "postgres#aaaa1111", Tool: "query", Tokens: 400, Calls: 5},
		{Day: today, Agent: "claude", Server: "postgres#bbbb2222", Tool: "query", Tokens: 400},
		{Day: today, Agent: "claude", Server: "github#cccc3333", Tool: "search", Tokens: 300},
	}
	labels := map[string]string{}
	for _, s := range buildReview(stats, nil, now).Servers {
		labels[s.ID] = s.Label
	}
	want := map[string]string{"postgres#aaaa1111": "postgres (aaaa1111)", "postgres#bbbb2222": "postgres (bbbb2222)", "github#cccc3333": "github"}
	if len(labels) != 3 || labels["postgres#aaaa1111"] != want["postgres#aaaa1111"] || labels["postgres#bbbb2222"] != want["postgres#bbbb2222"] || labels["github#cccc3333"] != "github" {
		t.Fatalf("labels %v", labels)
	}
}
