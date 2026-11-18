package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// debugLog captures the default logger at debug level for the test.
func debugLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func wantLogged(t *testing.T, buf *bytes.Buffer, substr string) {
	t.Helper()
	if !strings.Contains(buf.String(), substr) {
		t.Errorf("debug log lacks %q:\n%s", substr, buf)
	}
}

func TestCorruptStateIsLogged(t *testing.T) {
	sandbox(t)
	for name, tc := range map[string]struct {
		file string
		read func()
	}{
		"prefs":   {prefsFile(), func() { readPrefs() }},
		"release": {releaseFile(), func() { readRelease() }},
		"rules":   {allowRules.stateFile(), func() { allowRules.read() }},
	} {
		t.Run(name, func(t *testing.T) {
			buf := debugLog(t)
			if err := os.MkdirAll(filepath.Dir(tc.file), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(tc.file, []byte("{not json"), 0o600); err != nil {
				t.Fatal(err)
			}
			tc.read()
			wantLogged(t, buf, filepath.Base(tc.file))
		})
	}
}

func TestTranscriptScanErrors(t *testing.T) {
	sandbox(t)
	good := `{"type":"user","message":{"content":"first request"}}` + "\n"
	long := `{"type":"user","message":{"content":"` + strings.Repeat("x", 2<<20) + `"}}` + "\n"
	old := transcriptLineMax
	transcriptLineMax = 1 << 20
	t.Cleanup(func() { transcriptLineMax = old })
	buf := debugLog(t)
	// A line over the cap stops the scan: a snapshot of the part read would read as complete.
	if got := resumeSnapshot(writeTranscript(t, good+long+good)); got != "" {
		t.Errorf("truncated transcript gave a snapshot: %q", got)
	}
	wantLogged(t, buf, "transcript")
}

func TestMalformedToolInputLogged(t *testing.T) {
	sandbox(t)
	buf := debugLog(t)
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"a","name":"Edit","input":"oops"},{"type":"tool_use","id":"b","name":"Bash","input":"oops"},{"type":"tool_use","id":"c","name":"TodoWrite","input":"oops"}]}}` + "\n" +
		`{"type":"user","message":{"content":{"not":"blocks"}}}` + "\n"
	resumeSnapshot(writeTranscript(t, line))
	for _, what := range []string{"edited path", "command", "todos", "content blocks"} {
		wantLogged(t, buf, what)
	}
}

func TestClaudeUsageTruncatedSession(t *testing.T) {
	home := sandbox(t)
	old := transcriptLineMax
	transcriptLineMax = 1024
	t.Cleanup(func() { transcriptLineMax = old })
	buf := debugLog(t)
	proj := filepath.Join(home, ".claude", "projects", "-work")
	call := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"used"}}]}}` + "\n"
	for i := range 2 {
		write(t, filepath.Join(proj, fmt.Sprintf("ok%d.jsonl", i)), call)
	}
	write(t, filepath.Join(proj, "cut.jsonl"), call+strings.Repeat("x", 4096)+"\n")
	u := claudeUsage(home)
	if u.sessions != 2 {
		t.Errorf("sessions = %d, want 2: a cut transcript cannot show a skill unused", u.sessions)
	}
	wantLogged(t, buf, "cut.jsonl")
}

func TestAskReportsReadError(t *testing.T) {
	var out bytes.Buffer
	ds := []decision{{ID: "a", Default: "x", Options: []option{{Value: "x"}}}}
	_, err := ask(failingReader{}, &out, ds, false)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want the read error", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("boom") }

func TestHomeErrorLogged(t *testing.T) {
	sandbox(t)
	t.Setenv("HOME", "")
	buf := debugLog(t)
	if got := home(); got != "" {
		t.Errorf("home = %q", got)
	}
	wantLogged(t, buf, "home")
}

func TestStateFilesPrivateAndAtomic(t *testing.T) {
	sandbox(t)
	if err := allowRules.write(map[string][]string{"p": {"a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := newSetupToken(); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{allowRules.stateFile(), setupTokenFile()} {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want 0600", filepath.Base(f), info.Mode().Perm())
		}
		if _, err := os.Stat(f + ".tmp"); err == nil {
			t.Errorf("%s left a temp file", filepath.Base(f))
		}
	}
}
