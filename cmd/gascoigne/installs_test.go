package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func staleClaude(t *testing.T, global bool) {
	t.Helper()
	e := install{Agent: "claude", Global: global, Exe: "gascoigne"}
	if !global {
		e.Dir, _ = os.Getwd()
	}
	if err := writeInstalls([]install{e}); err != nil {
		t.Fatal(err)
	}
}

func TestRecordAndForgetInstalls(t *testing.T) {
	sandbox(t)
	for _, global := range []bool{true, false, false} { // the repeat replaces
		if err := recordInstall("claude", global, "gascoigne", false); err != nil {
			t.Fatal(err)
		}
	}
	if err := recordInstall("codex", true, "/bin/gascoigne", true); err != nil {
		t.Fatal(err)
	}
	list := readInstalls()
	if len(list) != 3 {
		t.Fatalf("want claude user + claude project + codex user, got %+v", list)
	}
	for _, e := range list {
		if e.Registration != registrationVersion || e.Global == (e.Dir != "") {
			t.Errorf("bad entry %+v", e)
		}
	}
	if err := forgetInstalls([]string{"claude"}); err != nil {
		t.Fatal(err)
	}
	if list := readInstalls(); len(list) != 1 || list[0].Agent != "codex" || !list[0].Proxy {
		t.Fatalf("after unwrap of claude: %+v", list)
	}
}

func TestSyncInstalls(t *testing.T) {
	const userSettings = ".claude/settings.json"
	tests := []struct {
		name     string
		prefs    string
		env      string
		global   bool
		wantSync bool
	}{
		{"not decided: nothing", "", "", true, false},
		{"autosync off", "off", "", true, false},
		{"autosync on", "on", "", true, true},
		{"env turns it off", "on", "off", true, false},
		{"project install is left alone by hooks", "on", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := sandbox(t)
			t.Setenv("GASCOIGNE_AUTOSYNC", tt.env)
			if err := saveAutosync(tt.prefs); err != nil {
				t.Fatal(err)
			}
			staleClaude(t, tt.global)
			lines := syncInstalls()
			file := userSettings
			if tt.global {
				file = filepath.Join(home, userSettings)
			}
			data, _ := os.ReadFile(file)
			synced := strings.Contains(string(data), "PostToolUse")
			if synced != tt.wantSync || (len(lines) > 0) != tt.wantSync {
				t.Fatalf("synced = %v (lines %v), want %v", synced, lines, tt.wantSync)
			}
			if got := readInstalls()[0].Registration; (got == registrationVersion) != tt.wantSync {
				t.Errorf("recorded registration = %d", got)
			}
		})
	}
}

func TestSyncInstallsKeepsProxyAndTakesLocks(t *testing.T) {
	sandbox(t)
	if err := saveAutosync("on"); err != nil {
		t.Fatal(err)
	}
	staleClaude(t, true)
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(stateDir(), "installs.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if lines := syncInstalls(); len(lines) != 0 {
		t.Fatalf("another process holds the lock: %v", lines)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if lines := syncInstalls(); len(lines) != 1 {
		t.Fatalf("a stale lock is taken over: %v", lines)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Error("lock left behind")
	}
	if lines := syncInstalls(); len(lines) != 0 {
		t.Errorf("a current install is not synced again: %v", lines)
	}
}

func TestSessionStartAnnouncesUpdatedHooks(t *testing.T) {
	sandbox(t)
	h := withGuide(func(context.Context, *common, hookInput, string) (any, error) { return nil, nil })
	reply, err := h(t.Context(), &common{}, hookInput{HookEventName: "SessionStart", syncNote: "gascoigne: claude: updated its gascoigne hooks. Restart the agent to use new hooks."}, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := reply.(map[string]any)["hookSpecificOutput"].(map[string]any)["additionalContext"].(string)
	if !strings.HasPrefix(ctx, "gascoigne: claude: updated") || !strings.Contains(ctx, "gascoigne shrinks") {
		t.Errorf("context:\n%s", ctx)
	}
}
