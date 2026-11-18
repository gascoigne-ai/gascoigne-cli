package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// characterizationTranscript is a transcript of the shape Claude Code writes: attachments, a hook,
// streamed repeats, a subagent line, a synthetic line, an idle pause, a model switch, a 1-hour
// cache write and a compaction.
func characterizationTranscript() string {
	at := func(minute int) string {
		return time.Date(2026, 10, 10, 10, 0, 0, 0, time.UTC).Add(time.Duration(minute) * time.Minute).Format(time.RFC3339)
	}
	attachment := func(minute int, body string) string {
		return fmt.Sprintf(`{"type":"attachment","sessionId":"s1","cwd":"/work/shop","timestamp":%q,"attachment":%s}`, at(minute), body)
	}
	return strings.Join([]string{
		`{"type":"user","message":{"content":"hi"}}`,
		attachment(0, `{"type":"instructions","files":["CLAUDE.md contents of the instruction files, long enough to count"]}`),
		attachment(0, `{"type":"hook_success","hookEvent":"UserPromptSubmit","command":"/opt/hooks/nudge.sh --quiet","content":"remember to run the tests before you finish"}`),
		lineAt("m1", at(1), "claude-opus-5-5", 10, 0, 40_000, 100, ""),
		lineAt("m2", at(2), "claude-opus-5-5", 10, 40_000, 20_000, 200, ""),
		lineAt("m2", at(2), "claude-opus-5-5", 10, 40_000, 20_000, 250, ""),
		lineAt("sub", at(3), "claude-opus-5-5", 10, 90_000, 0, 50, `,"isSidechain":true`),
		lineAt("syn", at(3), "<synthetic>", 0, 0, 0, 0, ""),
		`not json at all`,
		attachment(10, `{"type":"deferred_tools_delta"}`),
		attachment(10, `{"type":"skill_listing","content":"a listing of the skills that are available"}`),
		lineAt("m3", at(10), "claude-opus-5-5", 10, 5_000, 75_000, 300, ""),
		lineAt("m4", at(90), "claude-opus-5-5", 10, 0, 80_000, 300, `,"x":1`),
		attachment(95, `{"type":"hook_additional_context","hookEvent":"PostToolUse","content":["extra context for the model"]}`),
		lineAt("m5", at(96), "claude-sonnet-5-5", 10, 80_000, 70_000, 400, ""),
		lineAt("m6", at(97), "claude-sonnet-5-5", 10, 150_000, 60_000, 500, ""),
		lineAt("m7", at(98), "claude-sonnet-5-5", 10, 200_000, 10_000, 500, ""),
		lineAt("m8", at(99), "claude-sonnet-5-5", 10, 0, 40_000, 100, ""), // the context fell: a compaction
		lineAt("m9", at(100), "claude-sonnet-5-5", 10, 40_000, 2_000, 100, ""),
		lineAt("m10", at(101), "claude-sonnet-5-5", 10, 42_000, 1_000, 100, ""), // sonnet leads, so the session model is not a tie
		"",
	}, "\n")
}

// The whole report of a recorded-shape session, as JSON, as text on an API plan and as text on a
// flat plan, pinned so that splitting context.go cannot change one byte of it.
func TestContextReportCharacterization(t *testing.T) {
	home := sandbox(t)
	t.Cleanup(func() { cost.SetBilling(""); cost.SetDetector(nil) })
	local := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = local })

	dir := filepath.Join(home, "projects", "shop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "s1.jsonl")
	if err := os.WriteFile(path, []byte(characterizationTranscript()), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	load := func(mode string) contextReport {
		cost.SetDetector(func(string) cost.Detection { return cost.Detection{Mode: mode} })
		report, err := loadContext(filepath.Join(home, "projects"), 14*24*time.Hour, now)
		if err != nil {
			t.Fatal(err)
		}
		return report
	}
	api := load(cost.BillingAPI)
	var text strings.Builder
	printContext(&text, api)
	asJSON, err := json.MarshalIndent(api, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	var flat strings.Builder
	printContext(&flat, load(cost.BillingSubscription))

	for name, got := range map[string]string{"json": string(asJSON), "text": text.String(), "flat": flat.String()} {
		want := characterizationWant[name]
		if got != want {
			t.Errorf("%s output changed:\n%s", name, got)
		}
	}
}
