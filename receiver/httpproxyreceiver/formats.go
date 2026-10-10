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
	changed := false
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
// images and other blocks pass.
func replaceContent(ctx context.Context, r *Receiver, c call, content any, m meta) (any, bool) {
	switch v := content.(type) {
	case string:
		out := r.result(ctx, c, v, m)
		return out, out != v
	case []any:
		changed := false
		for _, bv := range v {
			b, _ := bv.(map[string]any)
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
