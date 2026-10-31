package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"maps"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/imgscale"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
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

// hintRe finds output gascoigne already optimized: exec's hint and the MCP
// server's and proxy's.
var hintRe = lazyre.New(`\[(gascoigne: )?optimized \d+→\d+ tokens · full: `)

// withResults answers Claude's PostToolUse events; other events go to h.
func withResults(h hookFunc) hookFunc {
	return func(ctx context.Context, c *common, in hookInput, prefix string) (any, error) {
		if in.HookEventName != "PostToolUse" {
			return h(ctx, c, in, prefix)
		}
		out, err := optimizeResult(ctx, c, in)
		if err != nil || out == nil {
			return nil, err
		}
		return map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PostToolUse", "updatedToolOutput": out}}, nil
	}
}

// optimizeResult returns the tool's output with its text optimized, in the
// tool's own shape, or nil to leave it alone.
func optimizeResult(ctx context.Context, c *common, in hookInput) (any, error) {
	var resp any
	if err := json.Unmarshal(in.ToolResponse, &resp); err != nil {
		return nil, nil
	}
	name := in.ToolName
	switch {
	case name == "Bash":
		cmd, _ := in.ToolInput["command"].(string)
		if ownCommand(cmd) {
			return nil, nil
		}
		return replaceField(ctx, c, resp, "stdout", map[string]string{payload.AttrCmd: cmd})
	case name == "Grep":
		obj, _ := resp.(map[string]any)
		if obj["mode"] != "content" {
			return nil, nil
		}
		pattern, _ := in.ToolInput["pattern"].(string)
		return replaceField(ctx, c, resp, "content", map[string]string{payload.AttrCmd: "rg -n " + shellQuote(pattern)})
	case name == "Read":
		if out := foldRead(in, resp); out != nil {
			return out, nil
		}
		return readImage(ctx, c, resp)
	case isOtherMCP(name):
		return mcpResult(ctx, c, resp, mcptool.Attrs(name, in.ToolInput))
	}
	return nil, nil
}

// replaceField optimizes the string field of an object output.
func replaceField(ctx context.Context, c *common, resp any, field string, attrs map[string]string) (any, error) {
	obj, ok := resp.(map[string]any)
	if !ok {
		return nil, nil
	}
	text, _ := obj[field].(string)
	opt, err := optimizeText(ctx, c, text, attrs)
	if err != nil || opt == "" {
		return nil, err
	}
	out := maps.Clone(obj)
	out[field] = opt
	return out, nil
}

// mcpResult optimizes an MCP result: a string, a list of content blocks,
// or an object holding them under "content". Non-text blocks pass.
func mcpResult(ctx context.Context, c *common, resp any, attrs map[string]string) (any, error) {
	switch v := resp.(type) {
	case string:
		opt, err := optimizeText(ctx, c, v, attrs)
		if err != nil || opt == "" {
			return nil, err
		}
		return opt, nil
	case []any:
		changed := false
		out := make([]any, len(v))
		for i, b := range v {
			out[i] = b
			block, ok := b.(map[string]any)
			text, isText := block["text"].(string)
			if !ok || block["type"] != "text" || !isText {
				continue
			}
			opt, err := optimizeText(ctx, c, text, attrs)
			if err != nil {
				return nil, err
			}
			if opt != "" {
				nb := maps.Clone(block)
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
		opt, err := mcpResult(ctx, c, content, attrs)
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
func optimizeText(ctx context.Context, c *common, text string, attrs map[string]string) (string, error) {
	if strings.TrimSpace(text) == "" || hintRe.MatchString(text) {
		return "", nil
	}
	run := *c
	run.attrs = maps.Clone(c.attrs)
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
func readImage(ctx context.Context, c *common, resp any) (any, error) {
	obj, _ := resp.(map[string]any)
	file, _ := obj["file"].(map[string]any)
	data, _ := file["base64"].(string)
	if obj["type"] != "image" || data == "" {
		return nil, nil
	}
	maxPixels, err := c.maxImagePixels()
	if err != nil || maxPixels <= 0 {
		return nil, err
	}
	mt, _ := file["type"].(string)
	out, before, after, ok := imgscale.Shrink(data, mt, maxPixels)
	if !ok {
		return nil, nil
	}
	if err := c.keepImage(ctx, data, mt, before, after); err != nil {
		return nil, err
	}
	nf := maps.Clone(file)
	nf["base64"] = out
	if dims, ok := file["dimensions"].(map[string]any); ok {
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
