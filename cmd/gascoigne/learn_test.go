package main

import (
	"os"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

func run(session, cmd string, out component.Outcome, tokens int, rawID string) ledger.Entry {
	return ledger.Entry{Event: component.Event{Session: session, Command: cmd, Key: "cmd:" + cmd, Outcome: out, RawTokens: tokens, RawID: rawID}}
}

func TestLessons(t *testing.T) {
	var es []ledger.Entry
	for _, s := range []string{"s1", "s2"} { // go test run 3 times in each of two sessions
		for range 3 {
			es = append(es, run(s, "go test", component.Optimized, 5000, "r1"))
		}
	}
	for range 3 { // once-off habit in one session only: no lesson
		es = append(es, run("s1", "make lint", component.Optimized, 900, "r2"))
	}
	es = append(es, run("s1", "docker logs", component.Unchanged, 3000, ""), run("s2", "docker logs", component.Unchanged, 2000, ""), run("s2", "docker logs", component.Unchanged, 2500, ""))
	es = append(es, run("s1", "ls", component.Unchanged, 100, "")) // small: not worth a line
	es = append(es, ledger.Entry{Event: component.Event{Session: "s1", Command: "go build", Key: "cmd:go build", Outcome: component.Optimized, RawID: "r3", Route: "fold"}})

	got := lessons(es)
	if len(got) != 2 {
		t.Fatalf("want rerun of go test and big docker logs, got %+v", got)
	}
	// Most runs first: docker logs 3, go test 2 repeats.
	if got[0].Rule != "big" || got[0].Command != "docker logs" || !strings.Contains(got[0].Line, "about 2500 tokens") {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].Rule != "rerun" || got[1].Command != "go test" || got[1].Sessions != 2 || got[1].Runs != 2*(3-2) ||
		!strings.Contains(got[1].Line, "gascoigne raw <id> -grep PATTERN") {
		t.Errorf("second: %+v", got[1])
	}
	again := lessons(es)
	for i := range got {
		if got[i] != again[i] {
			t.Fatal("lessons must be deterministic")
		}
	}
}

func TestLessonsNeedRepeats(t *testing.T) {
	es := []ledger.Entry{run("s1", "go test", component.Optimized, 5000, "r1"), run("s1", "go test", component.Optimized, 5000, "r1")}
	if got := lessons(es); len(got) != 0 {
		t.Errorf("two runs are not a habit: %+v", got)
	}
	if got := lessons(nil); got != nil && len(got) != 0 {
		t.Errorf("nothing: %+v", got)
	}
}

func TestWriteLearnedRoundTrip(t *testing.T) {
	sandbox(t)
	ls := []lesson{{Line: "- first."}, {Line: "- second."}}
	write(t, "CLAUDE.local.md", "# Mine\n\nKeep this.\n")
	for range 2 { // a second apply replaces the block
		if _, err := writeLearned("CLAUDE.local.md", ls); err != nil {
			t.Fatal(err)
		}
	}
	got := read(t, "CLAUDE.local.md")
	if strings.Count(got, learnStart) != 1 || !strings.HasPrefix(got, "# Mine\n\nKeep this.\n\n"+learnStart) || !strings.Contains(got, "- second.") {
		t.Fatalf("file:\n%s", got)
	}
	if _, err := writeLearned("CLAUDE.local.md", nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, "CLAUDE.local.md"); got != "# Mine\n\nKeep this.\n" {
		t.Errorf("after remove: %q", got)
	}
	// A file only gascoigne wrote goes away with its block.
	if _, err := writeLearned("new.md", ls); err != nil {
		t.Fatal(err)
	}
	if _, err := writeLearned("new.md", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("new.md"); !os.IsNotExist(err) {
		t.Error("a file with only the learned block is deleted by remove")
	}
	if d, err := writeLearned("absent.md", nil); d != "" || err != nil {
		t.Errorf("removing from a missing file: %q %v", d, err)
	}
}
