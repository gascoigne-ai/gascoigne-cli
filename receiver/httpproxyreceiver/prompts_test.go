package httpproxyreceiver

import (
	"slices"
	"testing"
)

func TestInstructions(t *testing.T) {
	reminder := "<system-reminder>Contents of /x/CLAUDE.md: rules</system-reminder>"
	tests := []struct {
		name string
		doc  map[string]any
		want []string
	}{
		{"anthropic", map[string]any{
			"system": []any{map[string]any{"type": "text", "text": "You are an agent."}},
			"messages": []any{
				map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "hi " + reminder + " do it"}}},
				map[string]any{"role": "assistant", "content": "ok <system-reminder>not mine</system-reminder>"},
			}}, []string{"You are an agent.", reminder}},
		{"chat", map[string]any{"messages": []any{
			map[string]any{"role": "system", "content": "sys"}, map[string]any{"role": "developer", "content": "dev"},
			map[string]any{"role": "user", "content": "plain"}}}, []string{"sys", "dev"}},
		{"responses", map[string]any{"instructions": "be brief", "input": []any{
			map[string]any{"role": "developer", "content": []any{map[string]any{"type": "input_text", "text": "agents.md"}}}}},
			[]string{"be brief", "agents.md"}},
		{"gemini", map[string]any{"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": "gem"}}}}, []string{"gem"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := instructions(tt.doc); !slices.Equal(got, tt.want) {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRecordPromptsOncePerBlock(t *testing.T) {
	r, _, _, _, _ := setup(t)
	l := &memLedger{}
	r.ledger = l
	doc := map[string]any{"system": "You are an agent.", "messages": []any{
		map[string]any{"role": "user", "content": "<system-reminder>The following skills are available: x</system-reminder>"}}}
	r.recordPrompts(t.Context(), doc, meta{model: "m"})
	r.recordPrompts(t.Context(), doc, meta{model: "m"})
	var kinds []string
	for _, ev := range l.events {
		kinds = append(kinds, ev.Command+"/"+ev.Route+"/"+string(ev.Outcome))
	}
	if want := []string{"system prompt/prompt/unchanged", "skills/prompt/unchanged"}; !slices.Equal(kinds, want) {
		t.Fatalf("events %q, want %q", kinds, want)
	}
	if promptKind("<system-reminder>Contents of /p/CLAUDE.md (project instructions)") != "CLAUDE.md" ||
		promptKind("<system-reminder>todo list empty</system-reminder>") != "reminder" {
		t.Fatal("promptKind")
	}
}
