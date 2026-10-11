package httpproxyreceiver

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/tokens"
)

// skillHead starts the text Claude Code sends when a skill is invoked: the
// SKILL.md body, as a user text block, not a tool result.
const skillHead = "Base directory for this skill: "

var htmlComment = lazyre.New(`(?s)<!--.*?-->\n?`)

// skills records each invoked skill body once, keeping its text, and with skill_code_lines
// set, shrinks it without changing an instruction: HTML comments go and
// code blocks longer than skill_code_lines keep their first lines plus a
// pointer to read the rest by line range. The body is the same in every
// request, and so is its shrunk form (cached by hash): the prompt cache holds.
func (r *Receiver) skills(ctx context.Context, doc map[string]any, m meta) bool {
	changed := false
	for _, mv := range asList(doc["messages"]) {
		msg, _ := mv.(map[string]any)
		if msg["role"] != "user" {
			continue
		}
		if s, ok := msg["content"].(string); ok {
			if out := r.skill(ctx, s, m); out != s {
				msg["content"], changed = out, true
			}
			continue
		}
		for _, bv := range asList(msg["content"]) {
			b, _ := bv.(map[string]any)
			text, _ := b["text"].(string)
			if b["type"] != "text" {
				continue
			}
			if out := r.skill(ctx, text, m); out != text {
				b["text"], changed = out, true
			}
		}
	}
	return changed
}

// skill returns a skill body's text as forwarded; other text unchanged.
func (r *Receiver) skill(ctx context.Context, text string, m meta) string {
	if !strings.HasPrefix(text, skillHead) || r.ledger == nil {
		return text
	}
	key := "skill\x00" + cacheKey(call{name: "skill"}, text)
	if out, ok := r.cache.get(key); ok {
		return out
	}
	out := r.shrinkSkill(ctx, text, m)
	r.cache.put(key, out)
	// A shrunk body seen again passes as it is, unrecorded.
	r.cache.put("skill\x00"+cacheKey(call{name: "skill"}, out), out)
	return out
}

func (r *Receiver) shrinkSkill(ctx context.Context, text string, m meta) string {
	dir, _, _ := strings.Cut(strings.TrimPrefix(text, skillHead), "\n")
	name := path.Base(strings.TrimSpace(dir))
	p := payload.New(payload.Message, text)
	p.Set(payload.AttrTool, "Skill")
	ev := component.Event{TS: time.Now(), Agent: m.agent, Model: m.model, Pipeline: "proxy",
		Key: "skill:" + name, Route: "skill", Command: "skill " + name}
	// The body is kept like an original, shrunk or not, so gascoigne raw
	// and the dashboard show what the skill told the agent.
	id, err := r.ledger.SaveRaw(ctx, ev.Key, p)
	if err != nil {
		r.log.Warn("save skill", "skill", name, "err", err)
		return text
	}
	in := tokens.Estimate(text)
	out := text
	if r.cfg.SkillCodeLines > 0 {
		if shrunk := shrinkSkillText(text, r.cfg.SkillCodeLines, id); in-tokens.Estimate(shrunk) >= minSaving {
			out = shrunk
		}
	}
	ev.RawID, ev.Output = id, out
	ev.RawTokens, ev.OutTokens = in, tokens.Estimate(out)
	ev.Outcome = component.Unchanged
	ev.Reason = "skill body, sent with every later request (recorded once per distinct body)"
	if out != text {
		ev.Outcome, ev.Reason = component.Optimized, ""
		ev.Optimizers = []string{"skill"}
		ev.Steps = []component.Step{{Optimizer: "skill", Family: "core", RawTokens: in, OutTokens: ev.OutTokens}}
	}
	if err := r.ledger.Record(ctx, ev); err != nil {
		r.log.Warn("record skill", "skill", name, "err", err)
	}
	return out
}

// shrinkSkillText drops HTML comments and folds code blocks over max lines
// to their first max lines and a pointer naming the original's line range.
func shrinkSkillText(text string, max int, id string) string {
	lines := strings.SplitAfter(text, "\n")
	var b strings.Builder
	inComment := false
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		switch {
		case inComment:
			inComment = !strings.Contains(t, "-->")
			continue
		case strings.HasPrefix(t, "<!--"):
			if rest := htmlComment.ReplaceAllString(l, ""); rest != l {
				b.WriteString(rest)
			} else {
				inComment = true
			}
			continue
		case strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~"):
			end := i + 1
			for end < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[end]), t[:3]) {
				end++
			}
			body := lines[i+1 : min(end, len(lines))]
			b.WriteString(l)
			if len(body) <= max {
				for _, x := range body {
					b.WriteString(x)
				}
			} else {
				for _, x := range body[:max] {
					b.WriteString(x)
				}
				fmt.Fprintf(&b, "… %d more lines: gascoigne raw %s -lines %d-%d\n", len(body)-max, id, i+2+max, i+1+len(body))
			}
			if end < len(lines) {
				b.WriteString(lines[end])
			}
			i = end
			continue
		}
		b.WriteString(l)
	}
	return b.String()
}
