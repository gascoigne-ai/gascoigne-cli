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
			if anthropicBlock(ctx, receiver, calls, b, info) {
				changed = true
			}
		}
	}
	return changed
}

// anthropicBlock remembers a tool_use block in calls and shortens a tool_result block that
// answers a remembered call; true when it changed the block.
func anthropicBlock(ctx context.Context, receiver *Receiver, calls map[string]call, block map[string]any, info meta) bool {
	switch block["type"] {
	case "tool_use":
		id := str(block, "id")
		name := str(block, "name")
		input := obj(block, "input")
		calls[id] = call{name, input}
	case "tool_result":
		if block["is_error"] == true {
			return false
		}
		id := str(block, "tool_use_id")
		c, ok := calls[id]
		if !ok {
			return false
		}
		if content, ok := replaceContent(ctx, receiver, c, block["content"], info); ok {
			block["content"] = content
			return true
		}
	}
	return false
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
			if shortenBlock(ctx, receiver, toolCall, b, info) {
				changed = true
			}
		}
		return v, changed
	}
	return content, false
}

// shortenBlock downscales an image block or shortens a text block in place; true when it changed.
func shortenBlock(ctx context.Context, receiver *Receiver, toolCall call, block map[string]any, info meta) bool {
	if receiver.shrinkImages(ctx, block) {
		return true
	}
	text, ok := block["text"].(string)
	if !ok || block["type"] != "text" && block["type"] != "input_text" && block["type"] != "output_text" {
		return false
	}
	out := receiver.result(ctx, toolCall, text, info)
	if out == text {
		return false
	}
	block["text"] = out
	return true
}

// chatCompletions shortens role "tool" messages of a Chat Completions request.
func chatCompletions(ctx context.Context, receiver *Receiver, doc map[string]any, info meta) bool {
	calls := map[string]call{}
	changed := false
	for msg := range messages(doc, "messages") {
		for tc := range objects(msg["tool_calls"]) {
			fn := obj(tc, "function")
			id := str(tc, "id")
			name := str(fn, "name")
			args := str(fn, "arguments")
			calls[id] = call{name, arguments(args)}
		}
		if msg["role"] != "tool" {
			continue
		}
		id := str(msg, "tool_call_id")
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
		id := str(item, "call_id")
		switch item["type"] {
		case "function_call":
			name := str(item, "name")
			args := str(item, "arguments")
			calls[id] = call{name, arguments(args)}
		case "custom_tool_call":
			name := str(item, "name")
			input := str(item, "input")
			calls[id] = call{name, map[string]any{"command": input}}
		case "local_shell_call":
			action := obj(item, "action")
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
			if fc := obj(part, "functionCall"); fc != nil {
				name := str(fc, "name")
				id := str(fc, "id")
				args := obj(fc, "args")
				calls = append(calls, &geminiCall{call: call{name, args}, id: id})
				continue
			}
			fr := obj(part, "functionResponse")
			resp := obj(fr, "response")
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
	name := str(fr, "name")
	id := str(fr, "id")
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
	src := obj(block, "source")
	data := str(src, "data")
	if src["type"] != "base64" || data == "" {
		return false
	}
	mt := str(src, "media_type")
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
