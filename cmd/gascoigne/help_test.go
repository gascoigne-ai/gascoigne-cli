package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverviewListsCommands(t *testing.T) {
	t.Setenv("GASCOIGNE_CONFIG", filepath.Join(t.TempDir(), "none.yaml"))
	var b bytes.Buffer
	printOverview(&b)
	for _, c := range commandList {
		listed := strings.Contains(b.String(), "\n  "+c.name+" ") || strings.Contains(b.String(), "\n  "+c.name+"\n")
		if listed != (c.group != "" && available(c)) {
			t.Errorf("%s: listed %v, group %q", c.name, listed, c.group)
		}
	}
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("colors in output that is not a terminal")
	}
}

func TestExperimentalCommands(t *testing.T) {
	sandbox(t) // the saved setup choices (prefs.json) count too: not the real ones
	for _, tc := range []struct {
		name, env, config string
		want              bool
	}{
		{"off", "", "", false},
		{"env names another", "other", "", false},
		{"env names it", "other,prompts", "", true},
		{"env all", "all", "", true},
		{"config names it", "", "experimental: [prompts]\n", true},
		{"config all", "", "experimental: [all]\n", true},
		{"config names another", "", "experimental: [other]\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(cfg, []byte(tc.config), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GASCOIGNE_CONFIG", cfg)
			t.Setenv("GASCOIGNE_EXPERIMENTAL", tc.env)
			if _, ok := lookup("prompts"); ok != tc.want {
				t.Errorf("lookup(prompts) = %v, want %v", ok, tc.want)
			}
			var b bytes.Buffer
			printOverview(&b)
			if listed := strings.Contains(b.String(), "\n  prompts "); listed != tc.want {
				t.Errorf("overview lists prompts: %v, want %v", listed, tc.want)
			}
		})
	}
	t.Setenv("GASCOIGNE_CONFIG", filepath.Join(t.TempDir(), "none.yaml"))
	t.Setenv("GASCOIGNE_EXPERIMENTAL", "")
	if err := unknownCommand("prompts"); !strings.Contains(err.Error(), "GASCOIGNE_EXPERIMENTAL=prompts") {
		t.Errorf("disabled command error = %v, want the env to set", err)
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	sandbox(t)
	for _, c := range commandList {
		if c.name == "help" {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(t.Context(), []string{"-h"}); !errors.Is(err, flag.ErrHelp) {
				t.Fatalf("-h: %v", err)
			}
		})
	}
}

func TestCommandPageSplitsSharedFlags(t *testing.T) {
	var c common
	fs := newFlags("exec")
	c.register(fs)
	fs.Bool("raw", false, "bypass optimization")
	var b bytes.Buffer
	printCommand(&b, "exec", fs)
	out := b.String()
	own, shared := strings.Index(out, "\nFLAGS"), strings.Index(out, "\nSHARED FLAGS")
	if own < 0 || shared < own {
		t.Fatalf("sections missing or out of order:\n%s", out)
	}
	if !strings.Contains(out[own:shared], "-raw") || strings.Contains(out[own:shared], "-config") {
		t.Errorf("own flags section:\n%s", out[own:shared])
	}
	if !strings.Contains(out[shared:], "-config file") {
		t.Errorf("shared flag value name:\n%s", out[shared:])
	}
}

func TestSuggest(t *testing.T) {
	for in, want := range map[string]string{"isntall": "install", "exe": "exec", "dash": "dashboard", "gian": "gain", "zzzzzz": ""} {
		if got := suggest(in); got != want {
			t.Errorf("suggest(%q) = %q, want %q", in, got, want)
		}
	}
}
