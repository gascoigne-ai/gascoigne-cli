package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func levels(fs []finding) string {
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.level + ": " + f.text + "\n")
	}
	return b.String()
}

func TestCheckInstall(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER_ADDR", "127.0.0.1:1") // nothing listens: no real server in the report
	home, _ := os.UserHomeDir()
	if _, err := addClaude("gascoigne", true); err != nil {
		t.Fatal(err)
	}
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	e := readInstalls()[0]
	if f := checkInstall(e); f.level != "ok" {
		t.Fatalf("healthy: %+v", f)
	}

	// Lost: someone removed the hook.
	os.Remove(filepath.Join(home, ".claude/settings.json"))
	lost := checkInstall(e)
	if lost.level != "problem" || !strings.Contains(lost.text, "hook is missing") || lost.fix == nil {
		t.Fatalf("lost hook: %+v", lost)
	}
	if err := lost.fix(); err != nil {
		t.Fatal(err)
	}
	if f := checkInstall(readInstalls()[0]); f.level != "ok" {
		t.Fatalf("after fix: %+v", f)
	}

	// Stale: registered by an older gascoigne.
	old := readInstalls()[0]
	old.Registration = registrationVersion - 1
	stale := checkInstall(old)
	if stale.level != "problem" || !strings.Contains(stale.text, "older gascoigne") || stale.fix == nil {
		t.Fatalf("stale: %+v", stale)
	}
	if err := stale.fix(); err != nil {
		t.Fatal(err)
	}
	if got := readInstalls()[0].Registration; got != registrationVersion {
		t.Errorf("fix records the current registration, got %d", got)
	}

	// A project elsewhere cannot be written from here.
	elsewhere := install{Agent: "claude", Dir: t.TempDir(), Exe: "gascoigne", Registration: registrationVersion}
	if f := checkInstall(elsewhere); f.level != "problem" || f.fix != nil || !strings.Contains(f.text, elsewhere.Dir) {
		t.Fatalf("project elsewhere: %+v", f)
	}
}

func TestDiagnose(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER_ADDR", "127.0.0.1:1")
	got := levels(diagnose())
	for _, want := range []string{"warn: gascoigne", "not on PATH", "background server: stopped", "usage text for agents: full", "choices waiting for you: Fewer approval prompts; Update hooks after an upgrade. Answer them with: gascoigne setup", "no coding agent found"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	t.Setenv("GASCOIGNE_SERVER", "off")
	if got := levels(diagnose()); !strings.Contains(got, "GASCOIGNE_SERVER=off") {
		t.Errorf("server off not reported:\n%s", got)
	}
}

func TestClaudeSkillsRoundTrip(t *testing.T) {
	home := sandbox(t)
	mine := filepath.Join(home, ".claude/skills/gascoigne-doctor/SKILL.md")
	write(t, mine, "my own skill\n")
	done, err := addClaudeSkills(true)
	if err == nil {
		t.Fatal("a user's file of the same name must be reported, not replaced")
	}
	if got := read(t, mine); got != "my own skill\n" {
		t.Errorf("user's skill overwritten: %q", got)
	}
	if len(done) != 1 || !strings.Contains(done[0], "gascoigne-stats") {
		t.Fatalf("the other skill is still written: %v", done)
	}
	stats := filepath.Join(home, ".claude/skills/gascoigne-stats/SKILL.md")
	if got := read(t, stats); !strings.Contains(got, "disable-model-invocation: true") || !strings.Contains(got, skillMarker) {
		t.Errorf("skill:\n%s", got)
	}
	if done, _ := addClaudeSkills(true); len(done) != 0 {
		t.Errorf("a second install changes nothing: %v", done)
	}
	if _, err := removeClaudeSkills(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stats); !os.IsNotExist(err) {
		t.Error("gascoigne's skill stays after unwrap")
	}
	if got := read(t, mine); got != "my own skill\n" {
		t.Errorf("unwrap touched the user's skill: %q", got)
	}
}
