package httpproxyreceiver

import (
	"fmt"
	"strings"
	"testing"
)

func TestShrinkSkillText(t *testing.T) {
	code := ""
	for i := 1; i <= 6; i++ {
		code += fmt.Sprintf("step %d\n", i)
	}
	in := "Base directory for this skill: /s/verify\n\n# Verify\n<!-- maintainer note -->\nRun it.\n<!--\nmulti\n-->\n```sh\n" + code + "```\nDone.\n"
	want := "Base directory for this skill: /s/verify\n\n# Verify\nRun it.\n```sh\nstep 1\nstep 2\n… 4 more lines: gascoigne raw ab12 -lines 12-15\n```\nDone.\n"
	if got := shrinkSkillText(in, 2, "ab12"); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	// The pointer's range is the original's lines.
	lines := strings.Split(in, "\n")
	if lines[11] != "step 3" || lines[14] != "step 6" {
		t.Fatalf("range points at %q..%q", lines[11], lines[14])
	}
	if short := "Base directory for this skill: /s/x\n```\na\n```\n"; shrinkSkillText(short, 2, "id") != short {
		t.Fatal("short code block changed")
	}
}

func TestSkillsInRequest(t *testing.T) {
	r, _, _, _, _ := setup(t)
	l := &memLedger{raw: map[string]string{}}
	r.ledger = l
	body := "Base directory for this skill: /s/big\n```\n" + strings.Repeat("echo line of a long example script\n", 200) + "```\n"
	doc := map[string]any{"messages": []any{
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": body}}},
		map[string]any{"role": "user", "content": "plain question"},
	}}
	if !r.skills(t.Context(), doc, meta{}) {
		t.Fatal("skill body not shrunk")
	}
	got := doc["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(got, "… 170 more lines: gascoigne raw ") || len(got) >= len(body) {
		t.Fatalf("shrunk body:\n%s", got)
	}
	r.skills(t.Context(), doc, meta{}) // cached: same bytes, no second event
	if len(l.events) != 1 || l.events[0].Command != "skill big" || l.events[0].Route != "skill" || l.raw[l.events[0].RawID] != body {
		t.Fatalf("events %+v", l.events)
	}
}
