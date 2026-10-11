package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/configs"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
)

func TestUpgradeConfig(t *testing.T) {
	tests := []struct {
		name     string
		old      string
		keep     []string // in the upgraded file
		dropped  []string
		template bool // the upgrade is the bare template
	}{
		{name: "comments only", old: "## an older template\n# optimizers:\n#   grep: {}\n", template: true},
		{name: "parent without children", old: "optimizers:\n", template: true},
		{name: "settings kept", old: "optimizers:\n  grep: { max_per_file: 50 }\n",
			keep: []string{"max_per_file: 50"}},
		{name: "unknown field dropped", old: "optimizers:\n  grep:\n    max_per_file: 50\n    gone_option: 3\n",
			keep: []string{"max_per_file: 50"}, dropped: []string{"optimizers.grep.gone_option"}},
		{name: "unknown top-level key dropped", old: "telemetry: true\nfilters:\n  minsize: { min_tokens: 10 }\n",
			keep: []string{"min_tokens: 10"}, dropped: []string{"telemetry"}},
		{name: "only unknown settings", old: "telemetry: true\n", dropped: []string{"telemetry"}, template: true},
		{name: "own component instance kept", old: "optimizers:\n  grep/strict: { max_per_file: 5 }\n",
			keep: []string{"grep/strict:", "max_per_file: 5"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, dropped, err := upgradeConfig([]byte(tt.old))
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(dropped, tt.dropped) {
				t.Errorf("dropped %q, want %q", dropped, tt.dropped)
			}
			if tt.template != (string(out) == string(configs.User)) {
				t.Errorf("bare template = %v, want %v", !tt.template, tt.template)
			}
			if !strings.HasPrefix(string(out), string(configs.User)) {
				t.Error("upgrade does not start with this version's template")
			}
			for _, k := range tt.keep {
				if !strings.Contains(string(out), k) {
					t.Errorf("lost %q:\n%s", k, out[len(configs.User):])
				}
			}
			again, _, err := upgradeConfig(out)
			if err != nil || string(again) != string(out) {
				t.Errorf("not idempotent (err %v):\n%s", err, again[len(configs.User):])
			}
			data, err := overlay(configs.Default, out)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := config.Parse(data); err != nil {
				t.Errorf("upgraded config does not load: %v", err)
			}
		})
	}
}

func TestUpgradeConfigRefusesBrokenValues(t *testing.T) {
	if _, _, err := upgradeConfig([]byte("router: 5\n")); err == nil {
		t.Fatal("a value that cannot load was carried over")
	}
}

// userConfig points the user config dir at a temp dir holding data.
func userConfig(t *testing.T, data string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("GASCOIGNE_CONFIG", "")
	path := filepath.Join(home, "gascoigne", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStaleTemplateRefreshed(t *testing.T) {
	path := userConfig(t, "## an older template\n# optimizers:\n#   grep: {}\n")
	if _, err := configData(""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(configs.User) {
		t.Fatal("a template with no settings was not brought up to date")
	}
	if !fileExists(path + ".bak") {
		t.Fatal("previous file not kept")
	}
}

func TestConfigWithSettingsLeftAlone(t *testing.T) {
	const own = "optimizers:\n  grep: { max_per_file: 50 }\n"
	path := userConfig(t, own)
	if _, err := configData(""); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != own {
		t.Fatal("a config with settings was rewritten without -upgrade")
	}
}

func TestUnknownSettingNamesUpgrade(t *testing.T) {
	userConfig(t, "optimizers:\n  grep: { gone_option: 3 }\n")
	_, err := configData("")
	if err == nil || !strings.Contains(err.Error(), "optimizers.grep.gone_option") || !strings.Contains(err.Error(), "config -upgrade") {
		t.Fatalf("want an error naming the setting and the fix, got %v", err)
	}
}

func TestUpgradeCmd(t *testing.T) {
	path := userConfig(t, "optimizers:\n  grep:\n    max_per_file: 50\n    gone_option: 3\n")
	if err := upgradeCmd(""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), "gone_option: 3") || !strings.Contains(string(got), "max_per_file: 50") {
		t.Fatalf("upgrade result:\n%s", got[len(configs.User):])
	}
	if _, err := configData(""); err != nil {
		t.Fatalf("upgraded config does not load: %v", err)
	}
}
