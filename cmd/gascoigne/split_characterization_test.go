package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// Characterization of commands that were split into smaller functions: the
// output is pinned so the split changes none of it.

func TestCmdTokensFile(t *testing.T) {
	sandbox(t)
	file := filepath.Join(t.TempDir(), "in.txt")
	write(t, file, "hello world\nsecond line here\n")
	var err error
	out := capture(t, func() {
		err = cmdTokens(t.Context(), []string{"-exact=false", "-model", "claude-sonnet-5-5,nope", "-rereads", "3", file})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"29 bytes · 2 lines\n\n",
		"tokenizer         tokens  basis\n",
		"bytes/4                8  rule of thumb; overcounts whitespace, undercounts Claude\n",
		"\nmodel                    tokens     one read   in context\n",
		"nope                 unknown model\n",
		"\none read: cache write (or plain input); in context: plus 3 cache reads, as an agent rereads a tool result\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "claude-sonnet-5-5            11      $0.0000      $0.0000\n") {
		t.Errorf("model row missing:\n%s", out)
	}
}

func TestCmdTokensMissingFile(t *testing.T) {
	sandbox(t)
	if err := cmdTokens(t.Context(), []string{"-exact=false", filepath.Join(t.TempDir(), "none")}); err == nil {
		t.Fatal("want an error for a missing file")
	}
}

func TestCmdPromptsJSON(t *testing.T) {
	sandbox(t)
	dir := t.TempDir()
	line := "Always run the full test suite before you say that a change is finished.\n"
	write(t, filepath.Join(dir, "CLAUDE.md"), "# Rules\n"+line+line)
	var err error
	out := capture(t, func() { err = cmdPrompts(t.Context(), []string{"-json", dir}) })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Agent    string `json:"agent"`
		Tokens   int    `json:"tokens"`
		Sources  []struct{ Path string }
		Findings []struct{ Kind, Path, Detail string }
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if got.Agent != "claude" || got.Tokens == 0 || len(got.Sources) == 0 {
		t.Fatalf("report: %+v", got)
	}
	if len(got.Findings) != 1 || got.Findings[0].Kind != "duplicate" ||
		got.Findings[0].Detail != "1 lines repeated in the same file" {
		t.Fatalf("findings: %+v", got.Findings)
	}
}

func TestCmdPromptsRefusals(t *testing.T) {
	sandbox(t)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-agent", "vim"}, `unknown model ""`},
		{[]string{"-model", "nope"}, `unknown model "nope"`},
		{[]string{"-agent", "claude", "-model", "gpt-5.5"}, ""},
	} {
		var err error
		capture(t, func() { err = cmdPrompts(t.Context(), append(tc.args, "-json", t.TempDir())) })
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%v: %v", tc.args, err)
		case tc.want != "" && (err == nil || err.Error() != tc.want):
			t.Errorf("%v: got %v, want %s", tc.args, err, tc.want)
		}
	}
}

func TestCmdInstallRefusals(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	if err := cmdInstall(t.Context(), nil); err == nil || !strings.Contains(err.Error(), "no agent found on PATH") {
		t.Errorf("no agent: %v", err)
	}
	if err := cmdInstall(t.Context(), []string{"vim"}); err == nil || !strings.Contains(err.Error(), "vim") {
		t.Errorf("unknown agent: %v", err)
	}
}

func TestCmdWrapRefusalsAndPrint(t *testing.T) {
	sandbox(t)
	if err := cmdWrap(t.Context(), []string{"vim"}); err == nil || !strings.Contains(err.Error(), "vim") {
		t.Errorf("unknown agent: %v", err)
	}
	var err error
	out := capture(t, func() { err = cmdWrap(t.Context(), []string{"-print", "claude"}) })
	if err != nil || out == "" {
		t.Errorf("print: %v %q", err, out)
	}
}
