package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gascoigne-ai/gascoigne-cli/internal/hint"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"maps"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/imgscale"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/httpproxyreceiver"
)

// Claude Code lets a PostToolUse hook replace any tool's result
// (updatedToolOutput, which must keep the tool's output shape), so what
// the PreToolUse rewrite cannot reach is optimized after it ran: Bash
// commands left as they were (pipes, redirects, loops, background runs),
// Grep's content mode, every MCP server's results, remote and plugin
// ones included, and image Reads (downscaled). A text Read stays untouched:
// Claude numbers its lines itself, and dropped lines would shift the numbers
// it shows.

// strField is the string at key of a decoded JSON object; "" when absent or not a string.
func strField(fields map[string]any, key string) string {
	text, _ := fields[key].(string)
	return text
}

// objField is the object at key of a decoded JSON object; nil when absent or not an object.
// The reply keeps the tool's other fields, so objects stay maps.
func objField(fields map[string]any, key string) map[string]any {
	object, _ := fields[key].(map[string]any)
	return object
}

// textBlock is the text of an MCP content block {"type":"text","text":...}.
func textBlock(block any) (text string, ok bool) {
	fields, _ := block.(map[string]any)
	text, isText := fields["text"].(string)
	return text, isText && fields["type"] == "text"
}

// withResults answers Claude's PostToolUse events; other events go to h.
func withResults(hook hookFunc) hookFunc {
	return func(ctx context.Context, shared *common, in hookInput, prefix string) (any, error) {
		if in.HookEventName != "PostToolUse" {
			return hook(ctx, shared, in, prefix)
		}
		out, err := optimizeResult(ctx, shared, in)
		if err != nil || out == nil {
			return nil, err
		}
		return claudePostReply{HookSpecificOutput: claudePostOutput{HookEventName: "PostToolUse", UpdatedToolOutput: out}}, nil
	}
}

// claudePostReply answers claude's PostToolUse with the replaced result. The
// result stays untyped: it keeps the shape of whichever tool ran.
type claudePostReply struct {
	HookSpecificOutput claudePostOutput `json:"hookSpecificOutput"`
}

type claudePostOutput struct {
	HookEventName     string `json:"hookEventName"`
	UpdatedToolOutput any    `json:"updatedToolOutput"`
}

// optimizeResult returns the tool's output with its text optimized, in the
// tool's own shape, or nil to leave it alone.
func optimizeResult(ctx context.Context, shared *common, in hookInput) (any, error) {
	var resp any
	if err := json.Unmarshal(in.ToolResponse, &resp); err != nil {
		return nil, nil
	}
	name := in.ToolName
	switch {
	case name == "Bash":
		cmd := strField(in.ToolInput, "command")
		if ownCommand(cmd) {
			return nil, nil
		}
		return replaceField(ctx, shared, resp, "stdout", map[string]string{payload.AttrCmd: cmd})
	case name == "Grep":
		obj, _ := resp.(map[string]any)
		if obj["mode"] != "content" {
			return nil, nil
		}
		pattern := strField(in.ToolInput, "pattern")
		return replaceField(ctx, shared, resp, "content", map[string]string{payload.AttrCmd: "rg -n " + mcptool.ShellQuote(pattern)})
	case name == "Read":
		return readImage(ctx, shared, resp)
	case isOtherMCP(name):
		return mcpResult(ctx, shared, resp, mcptool.Attrs(name, in.ToolInput))
	}
	return nil, nil
}

// replaceField optimizes the string field of an object output.
func replaceField(ctx context.Context, shared *common, resp any, field string, attrs map[string]string) (any, error) {
	obj, ok := resp.(map[string]any)
	if !ok {
		return nil, nil
	}
	opt, err := optimizeText(ctx, shared, strField(obj, field), attrs)
	if err != nil || opt == "" {
		return nil, err
	}
	out := maps.Clone(obj)
	out[field] = opt
	return out, nil
}

// mcpResult optimizes an MCP result: a string, a list of content blocks,
// or an object holding them under "content". Non-text blocks pass.
func mcpResult(ctx context.Context, shared *common, resp any, attrs map[string]string) (any, error) {
	switch v := resp.(type) {
	case string:
		opt, err := optimizeText(ctx, shared, v, attrs)
		if err != nil || opt == "" {
			return nil, err
		}
		return opt, nil
	case []any:
		changed := false
		out := make([]any, len(v))
		for i, b := range v {
			out[i] = b
			text, ok := textBlock(b)
			if !ok {
				continue
			}
			opt, err := optimizeText(ctx, shared, text, attrs)
			if err != nil {
				return nil, err
			}
			if opt != "" {
				nb := maps.Clone(b.(map[string]any))
				nb["text"] = opt
				out[i], changed = nb, true
			}
		}
		if !changed {
			return nil, nil
		}
		return out, nil
	case map[string]any:
		content, ok := v["content"].([]any)
		if !ok {
			return nil, nil
		}
		opt, err := mcpResult(ctx, shared, content, attrs)
		if err != nil || opt == nil {
			return nil, err
		}
		out := maps.Clone(v)
		out["content"] = opt
		return out, nil
	}
	return nil, nil
}

// optimizeText runs text through the hook pipeline: the optimized text with
// its retrieve hint, or "" when nothing changed or it was optimized before.
func optimizeText(ctx context.Context, shared *common, text string, attrs map[string]string) (string, error) {
	if strings.TrimSpace(text) == "" || hint.Re.MatchString(text) {
		return "", nil
	}
	run := *shared
	run.attrs = maps.Clone(shared.attrs)
	if run.attrs == nil {
		run.attrs = attrFlag{}
	}
	maps.Copy(run.attrs, attrs)
	p, err := run.optimize(ctx, text, attrs[payload.AttrCmd])
	if err != nil || p == nil || p.Attrs[payload.AttrRawID] == "" {
		return "", err
	}
	return hinted(p), nil
}

// readImage downscales the screenshot an image Read returns when it is above
// the LLM proxy's max_image_pixels (one knob for both): Claude sends the image
// as it comes, so a hook is the only place to shrink it without the proxy. The
// shape stays: only file.base64 and the display size change. The original goes
// to the ledger first; if it cannot be kept the image stays.
func readImage(ctx context.Context, shared *common, resp any) (any, error) {
	obj, _ := resp.(map[string]any)
	file := objField(obj, "file")
	data := strField(file, "base64")
	if obj["type"] != "image" || data == "" {
		return nil, nil
	}
	maxPixels, err := shared.maxImagePixels()
	if err != nil || maxPixels <= 0 {
		return nil, err
	}
	mt := strField(file, "type")
	out, before, after, ok := imgscale.Shrink(data, mt, maxPixels)
	if !ok {
		return nil, nil
	}
	if err := shared.keepImage(ctx, data, mt, before, after); err != nil {
		return nil, err
	}
	nf := maps.Clone(file)
	nf["base64"] = out
	if dims := objField(file, "dimensions"); dims != nil {
		nf["dimensions"] = scaledDims(dims, out)
	}
	no := maps.Clone(obj)
	no["file"] = nf
	return no, nil
}

// scaledDims sets displayWidth/Height to the size of the downscaled image
// (Claude reports them when it resizes an image itself); the original stays.
func scaledDims(dims map[string]any, b64 string) map[string]any {
	raw, _ := base64.StdEncoding.DecodeString(b64)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return dims
	}
	out := maps.Clone(dims)
	out["displayWidth"], out["displayHeight"] = cfg.Width, cfg.Height
	return out
}

// maxImagePixels is receivers.httpproxy.max_image_pixels of the config.
func (c *common) maxImagePixels() (int, error) {
	cfg, err := c.load()
	if err != nil {
		return 0, err
	}
	rc, _ := httpproxyreceiver.NewFactory().DefaultConfig().(*httpproxyreceiver.Config)
	if err := config.DecodeNode(cfg.Receivers["httpproxy"], rc); err != nil {
		return 0, fmt.Errorf("receivers.httpproxy: %w", err)
	}
	return rc.MaxImagePixels, nil
}

// keepImage saves the original of a downscaled image in the ledger.
func (c *common) keepImage(ctx context.Context, data, mediaType string, before, after int) error {
	run := *c
	run.server, run.pipeline = true, "hook"
	svc, err := run.start(ctx)
	if svc != nil {
		defer svc.Shutdown(context.WithoutCancel(ctx))
	}
	if err != nil {
		return err
	}
	id := c.identity()
	ev := component.Event{TS: time.Now(), Session: id.Session, Agent: id.Agent, Model: id.Model, Pipeline: "hook"}
	return imgscale.Keep(ctx, svc.Ledger(), ev, data, mediaType, before, after)
}

// isOtherMCP: a tool of an MCP server other than gascoigne's own, whose
// results are already optimized. Claude and Codex name them
// mcp__server__tool, Gemini mcp_server_tool.
func isOtherMCP(name string) bool {
	switch {
	case strings.HasPrefix(name, "mcp__"):
		return !strings.HasPrefix(name, "mcp__gascoigne__")
	case strings.HasPrefix(name, "mcp_"):
		return !strings.HasPrefix(name, "mcp_gascoigne_")
	}
	return false
}

// mcpText is the text of an MCP result for agents whose hooks replace a
// result with plain text: a string, content blocks, or an object holding
// them under "content" (or Gemini's "llmContent"). Results with non-text
// blocks are left alone, so images are never dropped.
func mcpText(raw json.RawMessage) (string, bool) {
	var resp any
	if json.Unmarshal(raw, &resp) != nil {
		return "", false
	}
	if obj, ok := resp.(map[string]any); ok {
		resp = obj["content"]
		if resp == nil {
			resp = obj["llmContent"]
		}
	}
	switch v := resp.(type) {
	case string:
		return v, true
	case []any:
		var texts []string
		for _, b := range v {
			block, _ := b.(map[string]any)
			text, ok := block["text"].(string)
			if !ok || block["type"] != nil && block["type"] != "text" {
				return "", false
			}
			texts = append(texts, text)
		}
		return strings.Join(texts, "\n"), len(texts) > 0
	}
	return "", false
}
