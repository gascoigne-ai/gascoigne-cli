package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestOverviewListsCommands(t *testing.T) {
	var b bytes.Buffer
	printOverview(&b)
	for _, c := range commandList {
		listed := strings.Contains(b.String(), "\n  "+c.name+" ") || strings.Contains(b.String(), "\n  "+c.name+"\n")
		if listed != (c.group != "") {
			t.Errorf("%s: listed %v, group %q", c.name, listed, c.group)
		}
	}
	if strings.Contains(b.String(), "\x1b[") {
		t.Error("colors in output that is not a terminal")
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
