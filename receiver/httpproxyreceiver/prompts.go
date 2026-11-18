package httpproxyreceiver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// reminder is one <system-reminder> block agents put into user messages.
var reminder = lazyre.New(`(?s)<system-reminder>.*?</system-reminder>`)

// recordPrompts records the instruction text a request carries — system
// prompts, CLAUDE.md, skill lists, reminders — once per distinct block, so
// gain shows what instructions cost next to tool results. Nothing changes.
func (r *Receiver) recordPrompts(ctx context.Context, doc map[string]any, info meta) {
	if r.ledger == nil {
		return
	}
	for _, text := range instructions(doc) {
		r.recordPrompt(ctx, text, info)
	}
}

// instructions collects instruction text in every API's shape: Anthropic
// system and reminders in user text, chat/responses system and developer
// messages and instructions, Gemini systemInstruction.
func instructions(doc map[string]any) []string {
	var out []string
	add := func(value any) {
		for _, t := range texts(value) {
			if strings.TrimSpace(t) != "" {
				out = append(out, t)
			}
		}
	}
	add(doc["system"])
	add(doc["instructions"])
	if si := obj(doc, "systemInstruction"); si != nil {
		add(si["parts"])
	}
	for msg := range messages(doc, "messages", "input") {
		switch msg["role"] {
		case "system", "developer":
			add(msg["content"])
		case "user":
			for _, t := range texts(msg["content"]) {
				out = append(out, reminder.FindAllString(t, -1)...)
			}
		}
	}
	return out
}

// texts returns a content value's text: a string, or the text of each part
// of a list (text, input_text and Gemini parts).
func texts(value any) []string {
	if s, ok := value.(string); ok {
		return []string{s}
	}
	var out []string
	for part := range objects(value) {
		if t, ok := part["text"].(string); ok {
			out = append(out, t)
		}
	}
	return out
}

func (r *Receiver) recordPrompt(ctx context.Context, text string, info meta) {
	sum := sha256.Sum256([]byte(text))
	key := "prompt:" + hex.EncodeToString(sum[:8])
	if _, ok := r.cache.Get(key); ok {
		return
	}
	r.cache.Put(key, "")
	// The text is kept like an original (gascoigne raw <id>, the dashboard's
	// run view), so what an agent was told can be read back.
	p := payload.New(payload.Message, text)
	p.Set(payload.AttrTool, "prompt")
	id, err := r.ledger.SaveRaw(ctx, key, p)
	if err != nil {
		r.log.Warn("save instructions", "err", err)
	}
	n := tokens.Estimate(text)
	err = r.ledger.Record(ctx, component.Event{
		TS: time.Now(), Agent: info.agent, Model: info.model, Pipeline: "proxy", Key: key, Route: "prompt",
		Command: promptKind(text), RawTokens: n, OutTokens: n, RawID: id, Output: text, Outcome: component.Unchanged,
		Reason: "instruction text, sent with every request (recorded once per distinct block)",
	})
	if err != nil {
		r.log.Warn("record instructions", "err", err)
	}
}

// promptKind names instruction text for reports.
func promptKind(text string) string {
	switch {
	case strings.Contains(text, "CLAUDE.md") && strings.Contains(text, "Contents of "):
		return "CLAUDE.md"
	case strings.Contains(text, "skills are available"):
		return "skills"
	case strings.HasPrefix(text, "<system-reminder>"):
		return "reminder"
	}
	return "system prompt"
}
