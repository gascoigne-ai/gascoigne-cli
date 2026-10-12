package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// capture runs f and returns what it printed to stdout.
func capture(t *testing.T, f func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestUpgradeNothingToDo(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	if err := saveAllowLevel("none"); err != nil {
		t.Fatal(err)
	}
	if err := saveAutosync("off"); err != nil {
		t.Fatal(err)
	}
	var err error
	out := capture(t, func() { err = cmdUpgrade(t.Context(), nil) })
	if err != nil || !strings.Contains(out, "everything is up to date") {
		t.Fatalf("%v: %q", err, out)
	}
}

func TestUpgradeWritesStaleAndLostHooks(t *testing.T) {
	home := sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	_ = saveAllowLevel("none")
	_ = saveAutosync("off")
	if _, err := addClaude("gascoigne", true); err != nil {
		t.Fatal(err)
	}
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	list := readInstalls()
	list[0].Registration = registrationVersion - 1 // set up by an older version
	if err := writeInstalls(list); err != nil {
		t.Fatal(err)
	}
	os.Remove(home + "/.claude/skills/gascoigne-stats/SKILL.md") // a command the old version lacked

	dry := capture(t, func() { cmdUpgrade(t.Context(), []string{"-n"}) })
	if !strings.Contains(dry, "would update") || readInstalls()[0].Registration == registrationVersion {
		t.Fatalf("-n changes nothing and says what it would do: %q", dry)
	}
	if _, err := os.Stat(home + "/.claude/skills/gascoigne-stats/SKILL.md"); !os.IsNotExist(err) {
		t.Fatal("-n wrote a file")
	}

	out := capture(t, func() { cmdUpgrade(t.Context(), nil) })
	if !strings.Contains(out, "hooks: updated claude (all projects)") {
		t.Fatalf("output: %q", out)
	}
	if readInstalls()[0].Registration != registrationVersion {
		t.Error("the install is recorded as current")
	}
	if _, err := os.Stat(home + "/.claude/skills/gascoigne-stats/SKILL.md"); err != nil {
		t.Errorf("the command new in this version is written: %v", err)
	}
	if again := capture(t, func() { cmdUpgrade(t.Context(), nil) }); !strings.Contains(again, "everything is up to date") {
		t.Errorf("a second run: %q", again)
	}
}

func TestUpgradeMovesTheConfig(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	_ = saveAllowLevel("none")
	_ = saveAutosync("off")
	path, _ := configPath("")
	write(t, path, "optimizers:\n  grep:\n    max_per_file: 50\n  gone:\n    option: 1\n")
	out := capture(t, func() { cmdUpgrade(t.Context(), []string{"-n"}) })
	if !strings.Contains(out, "config: would upgrade") {
		t.Fatalf("dry run: %q", out)
	}
	if got := read(t, path); !strings.Contains(got, "gone") {
		t.Fatal("-n rewrote the config")
	}
	out = capture(t, func() { cmdUpgrade(t.Context(), nil) })
	if !strings.Contains(out, "config: upgraded") {
		t.Fatalf("output: %q", out)
	}
	if got := read(t, path); !strings.Contains(got, "max_per_file: 50") {
		t.Errorf("the user's setting is kept:\n%s", got)
	}
}

func TestUpdateQuestion(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	find := func() *decision {
		for _, d := range decisions(readPrefs()) {
			if d.ID == "update" {
				return &d
			}
		}
		return nil
	}
	if err := recordInstall("claude", true, "gascoigne", false); err != nil {
		t.Fatal(err)
	}
	if find() != nil {
		t.Fatal("current installs raise no question")
	}
	list := readInstalls()
	list[0].Registration = registrationVersion - 1
	if err := writeInstalls(list); err != nil {
		t.Fatal(err)
	}
	q := find()
	if q == nil || q.Current != "" || !strings.Contains(q.Text, "claude (all projects)") || !strings.Contains(q.Text, registrationNotes[registrationVersion]) {
		t.Fatalf("question: %+v", q)
	}
	if note := setupNudge(); !strings.Contains(note, "older than this version") {
		t.Errorf("session note: %q", note)
	}

	// Not now: remembered for this version, nothing rewritten.
	if _, err := applyAnswers(map[string]string{"update": "no"}); err != nil {
		t.Fatal(err)
	}
	if readInstalls()[0].Registration == registrationVersion {
		t.Fatal("no must not update")
	}
	if q := find(); q == nil || q.Current != "no" {
		t.Fatalf("declined for this version: %+v", q)
	}

	// Autosync on does it by itself, no question.
	if _, err := applyAnswers(map[string]string{"autosync": "on"}); err != nil {
		t.Fatal(err)
	}
	if find() != nil {
		t.Error("autosync on: nothing to ask")
	}
}
