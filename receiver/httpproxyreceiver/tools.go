package httpproxyreceiver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/schema"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// compactTools drops what the request's tool schemas say twice (see
// compress/schema), in every API's shape: Anthropic input_schema, OpenAI
// chat function.parameters, Responses parameters, Gemini
// functionDeclarations. The first request with a tool set records its saving.
func (r *Receiver) compactTools(ctx context.Context, doc map[string]any, m meta) bool {
	tools := asList(doc["tools"])
	if !r.cfg.CompactTools || len(tools) == 0 {
		return false
	}
	before, err := json.Marshal(tools)
	if err != nil {
		return false
	}
	changed := false
	for _, tv := range tools {
		for _, d := range declarations(tv) {
			name, _ := d["name"].(string)
			for _, key := range []string{"input_schema", "parameters", "parametersJsonSchema", "inputSchema"} {
				if s, ok := d[key].(map[string]any); ok && schema.Compact(name, s) {
					changed = true
				}
			}
		}
	}
	if changed {
		r.recordTools(ctx, string(before), doc["tools"], m)
	}
	return changed
}

// declarations are the objects of one tools entry that name a tool and hold
// its schema.
func declarations(v any) []map[string]any {
	t, _ := v.(map[string]any)
	if fn, ok := t["function"].(map[string]any); ok {
		return []map[string]any{fn}
	}
	if list := asList(t["functionDeclarations"]); len(list) > 0 {
		var out []map[string]any
		for _, d := range list {
			if m, ok := d.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return []map[string]any{t}
}

// recordTools records a tool set's saving once per distinct set: agents
// send the same tools with every request, and each one saves the same.
func (r *Receiver) recordTools(ctx context.Context, before string, tools any, m meta) {
	if r.ledger == nil {
		return
	}
	sum := sha256.Sum256([]byte(before))
	key := "tools:" + hex.EncodeToString(sum[:8])
	if _, ok := r.cache.get(key); ok {
		return
	}
	r.cache.put(key, "")
	after, err := json.Marshal(tools)
	if err != nil {
		return
	}
	p := payload.New(payload.JSON, before)
	p.Set(payload.AttrTool, "tools")
	id, err := r.ledger.SaveRaw(ctx, key, p)
	if err != nil {
		r.log.Warn("save tool definitions", "err", err)
		return
	}
	in, out := tokens.Estimate(before), tokens.Estimate(string(after))
	err = r.ledger.Record(ctx, component.Event{
		TS: time.Now(), Agent: m.agent, Model: m.model, Pipeline: "proxy", Key: key, Route: "tools",
		Command: "tool definitions", Optimizers: []string{"tools"}, RawTokens: in, OutTokens: out, RawID: id,
		Outcome: component.Optimized, Steps: []component.Step{{Optimizer: "tools", Family: "core", RawTokens: in, OutTokens: out}},
	})
	if err != nil {
		r.log.Warn("record tool definitions", "err", err)
	}
}
