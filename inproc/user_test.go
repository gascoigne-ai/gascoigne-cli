package inproc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A user file written for another version may hold settings this one lacks. The engine used to
// fail with a raw YAML decode error; now it names the settings and the fix, as the CLI does.
func TestUserFileWithUnknownSettingsIsNamed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "config.yaml")
	body := "optimizers:\n  grep:\n    not_a_setting: 1\nbogus_top: 2\n"
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GASCOIGNE_CONFIG", file)
	o := Options{Config: "extensions:\n  ledger:\n    path: " + filepath.Join(t.TempDir(), "l.db") + "\n"}
	_, err := New(context.Background(), o)
	if err == nil || !strings.Contains(err.Error(), "optimizers.grep.not_a_setting") ||
		!strings.Contains(err.Error(), "bogus_top") {
		t.Fatalf("error does not name the unknown settings: %v", err)
	}
}
