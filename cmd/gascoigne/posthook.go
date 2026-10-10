package main

import (
	"context"
	"encoding/json"
	"maps"
	"strings"

	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
)

// Claude Code lets a PostToolUse hook replace any tool's result
// (updatedToolOutput, which must keep the tool's output shape), so what
// the PreToolUse rewrite cannot reach is optimized after it ran: Bash
// commands left as they were (pipes, redirects, loops, background runs),
// Grep's content mode, and every MCP server's results, remote and plugin
// ones included. Read stays untouched: Claude numbers its lines itself, and
// dropped lines would shift the numbers it shows.

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
	case isOtherMCP(name):
		return mcpResult(ctx, c, resp, map[string]string{payload.AttrTool: name})
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
