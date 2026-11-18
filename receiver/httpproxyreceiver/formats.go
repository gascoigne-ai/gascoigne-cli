package httpproxyreceiver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/imgscale"
)

// anthropic shortens tool_result blocks of a Messages request; the tool_use
// blocks they answer say which tool produced them.
func anthropic(ctx context.Context, receiver *Receiver, doc map[string]any, info meta) bool {
	calls := map[string]call{}
	changed := false
	for msg := range messages(doc, "messages") {
		for b := range objects(msg["content"]) {
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
				if content, ok := replaceContent(ctx, receiver, c, b["content"], info); ok {
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
func replaceContent(ctx context.Context, receiver *Receiver, toolCall call, content any, info meta) (any, bool) {
	switch v := content.(type) {
	case string:
		out := receiver.result(ctx, toolCall, v, info)
		return out, out != v
	case []any:
		changed := false
		for b := range objects(v) {
			if receiver.shrinkImages(ctx, b) {
				changed = true
				continue
			}
			text, ok := b["text"].(string)
			if !ok || b["type"] != "text" && b["type"] != "input_text" && b["type"] != "output_text" {
				continue
			}
			if out := receiver.result(ctx, toolCall, text, info); out != text {
				b["text"] = out
				changed = true
			}
		}
		return v, changed
	}
	return content, false
}

// chatCompletions shortens role "tool" messages of a Chat Completions request.
func chatCompletions(ctx context.Context, receiver *Receiver, doc map[string]any, info meta) bool {
	calls := map[string]call{}
	changed := false
	for msg := range messages(doc, "messages") {
		for tc := range objects(msg["tool_calls"]) {
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
			if content, ok := replaceContent(ctx, receiver, c, msg["content"], info); ok {
				msg["content"] = content
				changed = true
			}
		}
	}
	return changed
}

// responses shortens *_call_output items of a Responses request (codex).
func responses(ctx context.Context, receiver *Receiver, doc map[string]any, info meta) bool {
	calls := map[string]call{}
	changed := false
	for item := range messages(doc, "input") {
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
			if out, ok := replaceContent(ctx, receiver, c, item["output"], info); ok {
				item["output"] = out
				changed = true
			}
		}
	}
	return changed
}

// arguments decodes a tool call's JSON arguments; codex shells pass
// {"command": ["bash", "-lc", "go test ./..."]} or {"cmd": "..."}.
func arguments(raw string) map[string]any {
	var a map[string]any
	if json.Unmarshal([]byte(raw), &a) != nil {
		return nil
	}
	return normalize(a)
}

// normalize turns a shell call's argv or cmd into a command string.
func normalize(args map[string]any) map[string]any {
	if cmd, ok := args["cmd"].(string); ok {
		args["command"] = cmd
	}
	if argv, ok := args["command"].([]any); ok {
		parts := make([]string, 0, len(argv))
		for _, x := range argv {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
		if n := len(parts); n >= 3 && (parts[n-2] == "-lc" || parts[n-2] == "-c") {
			args["command"] = parts[n-1]
		} else {
			args["command"] = strings.Join(parts, " ")
		}
	}
	return args
}

// gemini shortens functionResponse parts of a generateContent request.
// A response answers the call with its id, else the oldest unanswered call
// of its name (Gemini CLI puts tool output under response.output).
func gemini(ctx context.Context, receiver *Receiver, doc map[string]any, info meta) bool {
	var calls []*geminiCall
	changed := false
	for content := range messages(doc, "contents") {
		for part := range objects(content["parts"]) {
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
				if out, ok := replaceContent(ctx, receiver, c.call, resp[key], info); ok {
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
func (r *Receiver) shrinkImages(ctx context.Context, block map[string]any) bool {
	if r.cfg.MaxImagePixels <= 0 || block["type"] != "image" {
		return false
	}
	src, _ := block["source"].(map[string]any)
	data, _ := src["data"].(string)
	if src["type"] != "base64" || data == "" {
		return false
	}
	mt, _ := src["media_type"].(string)
	key := "image\x00" + cacheKey(call{name: mt}, data)
	out, ok := r.cache.Get(key)
	if !ok {
		var before, after int
		var shrunk bool
		out, before, after, shrunk = imgscale.Shrink(data, mt, r.cfg.MaxImagePixels)
		if shrunk {
			r.keepImage(ctx, data, mt, before, after)
		}
		r.cache.Put(key, out)
	}
	if out == data {
		return false
	}
	src["data"] = out
	return true
}
