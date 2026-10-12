package httpproxyreceiver

import (
	"context"
	"encoding/json"
	"strings"
)

// anthropic shortens tool_result blocks of a Messages request; the tool_use
// blocks they answer say which tool produced them.
func anthropic(ctx context.Context, r *Receiver, doc map[string]any, m meta) bool {
	msgs, _ := doc["messages"].([]any)
	calls := map[string]call{}
	changed := r.shapeSystem(doc)
	for _, mv := range msgs {
		msg, _ := mv.(map[string]any)
		blocks, _ := msg["content"].([]any)
		for _, bv := range blocks {
			b, _ := bv.(map[string]any)
			switch b["type"] {
			case "tool_use":
				id, _ := b["id"].(string)
				name, _ := b["name"].(string)
				input, _ := b["input"].(map[string]any)
				calls[id] = call{name, input}
			case "tool_result":
				if b["is_error"] == true {
					continue
				}
				id, _ := b["tool_use_id"].(string)
				c, ok := calls[id]
				if !ok {
					continue
				}
				if content, ok := replaceContent(ctx, r, c, b["content"], m); ok {
					b["content"] = content
					changed = true
				}
			}
		}
	}
	return changed
}

// replaceContent shortens a string or the text blocks of a content list;
// images above max_image_pixels are downscaled, other blocks pass.
func replaceContent(ctx context.Context, r *Receiver, c call, content any, m meta) (any, bool) {
	switch v := content.(type) {
	case string:
		out := r.result(ctx, c, v, m)
		return out, out != v
	case []any:
		changed := false
		for _, bv := range v {
			b, _ := bv.(map[string]any)
			if r.shrinkImages(ctx, b) {
				changed = true
				continue
			}
			text, ok := b["text"].(string)
			if !ok || b["type"] != "text" && b["type"] != "input_text" && b["type"] != "output_text" {
				continue
			}
			if out := r.result(ctx, c, text, m); out != text {
				b["text"] = out
				changed = true
			}
		}
		return v, changed
	}
	return content, false
}

// chatCompletions shortens role "tool" messages of a Chat Completions request.
func chatCompletions(ctx context.Context, r *Receiver, doc map[string]any, m meta) bool {
	msgs, _ := doc["messages"].([]any)
	calls := map[string]call{}
	changed := false
	for _, mv := range msgs {
		msg, _ := mv.(map[string]any)
		for _, tv := range asList(msg["tool_calls"]) {
			tc, _ := tv.(map[string]any)
			fn, _ := tc["function"].(map[string]any)
			id, _ := tc["id"].(string)
			name, _ := fn["name"].(string)
			args, _ := fn["arguments"].(string)
			calls[id] = call{name, arguments(args)}
		}
		if msg["role"] != "tool" {
			continue
		}
		id, _ := msg["tool_call_id"].(string)
		if c, ok := calls[id]; ok {
			if content, ok := replaceContent(ctx, r, c, msg["content"], m); ok {
				msg["content"] = content
				changed = true
			}
		}
	}
	return changed
}

// responses shortens *_call_output items of a Responses request (codex).
func responses(ctx context.Context, r *Receiver, doc map[string]any, m meta) bool {
	calls := map[string]call{}
	changed := false
	for _, iv := range asList(doc["input"]) {
		item, _ := iv.(map[string]any)
		id, _ := item["call_id"].(string)
		switch item["type"] {
		case "function_call":
			name, _ := item["name"].(string)
			args, _ := item["arguments"].(string)
			calls[id] = call{name, arguments(args)}
		case "custom_tool_call":
			name, _ := item["name"].(string)
			input, _ := item["input"].(string)
			calls[id] = call{name, map[string]any{"command": input}}
		case "local_shell_call":
			action, _ := item["action"].(map[string]any)
			calls[id] = call{"shell", normalize(map[string]any{"command": action["command"]})}
		case "function_call_output", "custom_tool_call_output", "local_shell_call_output":
			c, ok := calls[id]
			if !ok {
				continue
			}
			if out, ok := replaceContent(ctx, r, c, item["output"], m); ok {
				item["output"] = out
				changed = true
			}
		}
	}
	return changed
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// arguments decodes a tool call's JSON arguments; codex shells pass
// {"command": ["bash", "-lc", "go test ./..."]} or {"cmd": "..."}.
func arguments(s string) map[string]any {
	var a map[string]any
	if json.Unmarshal([]byte(s), &a) != nil {
		return nil
	}
	return normalize(a)
}

// normalize turns a shell call's argv or cmd into a command string.
func normalize(a map[string]any) map[string]any {
	if cmd, ok := a["cmd"].(string); ok {
		a["command"] = cmd
	}
	if argv, ok := a["command"].([]any); ok {
		parts := make([]string, 0, len(argv))
		for _, x := range argv {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		if n := len(parts); n >= 3 && (parts[n-2] == "-lc" || parts[n-2] == "-c") {
			a["command"] = parts[n-1]
		} else {
			a["command"] = strings.Join(parts, " ")
		}
	}
	return a
}

// gemini shortens functionResponse parts of a generateContent request.
// A response answers the call with its id, else the oldest unanswered call
// of its name (Gemini CLI puts tool output under response.output).
func gemini(ctx context.Context, r *Receiver, doc map[string]any, m meta) bool {
	var calls []*geminiCall
	changed := false
	for _, cv := range asList(doc["contents"]) {
		content, _ := cv.(map[string]any)
		for _, pv := range asList(content["parts"]) {
			part, _ := pv.(map[string]any)
			if fc, ok := part["functionCall"].(map[string]any); ok {
				name, _ := fc["name"].(string)
				id, _ := fc["id"].(string)
				args, _ := fc["args"].(map[string]any)
				calls = append(calls, &geminiCall{call: call{name, args}, id: id})
				continue
			}
			fr, _ := part["functionResponse"].(map[string]any)
			resp, _ := fr["response"].(map[string]any)
			c := answered(calls, fr)
			if c == nil || resp == nil {
				continue
			}
			for _, key := range []string{"output", "content"} {
				if out, ok := replaceContent(ctx, r, c.call, resp[key], m); ok {
					resp[key] = out
					changed = true
				}
			}
		}
	}
	return changed
}

type geminiCall struct {
	call
	id   string
	done bool
}

// answered finds the call a functionResponse answers, by id when it has
// one, else the oldest open call of its name, and marks it answered.
func answered(calls []*geminiCall, fr map[string]any) *geminiCall {
	name, _ := fr["name"].(string)
	id, _ := fr["id"].(string)
	for _, c := range calls {
		if !c.done && (id != "" && c.id == id || id == "" && c.name == name) {
			c.done = true
			return c
		}
	}
	return nil
}

// shrinkImages downscales an Anthropic base64 image block in place.
func (r *Receiver) shrinkImages(ctx context.Context, b map[string]any) bool {
	if r.cfg.MaxImagePixels <= 0 || b["type"] != "image" {
		return false
	}
	src, _ := b["source"].(map[string]any)
	data, _ := src["data"].(string)
	if src["type"] != "base64" || data == "" {
		return false
	}
	mt, _ := src["media_type"].(string)
	key := "image\x00" + cacheKey(call{name: mt}, data)
	out, ok := r.cache.get(key)
	if !ok {
		var before, after int
		var shrunk bool
		out, before, after, shrunk = shrink(data, mt, r.cfg.MaxImagePixels)
		if shrunk {
			r.keepImage(ctx, data, mt, before, after)
		}
		r.cache.put(key, out)
	}
	if out == data {
		return false
	}
	src["data"] = out
	return true
}
