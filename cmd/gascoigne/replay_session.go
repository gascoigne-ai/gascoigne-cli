package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// session is the conversation rebuilt from a transcript, as the client
// sends it: consecutive lines of one role merged into one message.
type session struct {
	messages []message
	model    string
	tools    map[string]string // tool_use id → tool name
	// What the previous request sent: the client's messages and the
	// proxy's, canonical JSON.
	prevOrig, prevFwd []string
	// Latest measure per tool result id.
	results map[string]resultTokens
	price   cost.Model
	tokens  map[string]int // memo: canonical message → tokens.Estimate
}

// message is one conversation message as the client sends it. Content stays a
// block list of untyped objects: blocks reach the proxy with every field they had.
// Content comes first so the encoded keys keep the sorted order of the old maps.
type message struct {
	Content []any  `json:"content"`
	Role    string `json:"role"`
}

type resultTokens struct {
	raw, out int
	folded   bool // points at an earlier result
}

type transcriptLine struct {
	Type             string `json:"type"`
	IsSidechain      bool   `json:"isSidechain"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	Message          *struct {
		ID      string `json:"id"`
		Role    string `json:"role"`
		Model   string `json:"model"`
		Content any    `json:"content"`
	} `json:"message"`
}

// add appends one transcript line to the conversation. A model turn that
// follows tool results first sends the request they ended. lastID is the id
// of the assistant message being assembled from consecutive lines.
func (s *session) add(ctx context.Context, proxy http.Handler, up *upstream, path string, rep *replayReport, line transcriptLine, lastID *string) error {
	content := blocks(line.Message.Content)
	switch {
	case line.IsCompactSummary:
		s.messages, s.prevOrig, s.prevFwd = []message{{Role: "user", Content: content}}, nil, nil
	case line.Type == "assistant":
		s.model = cmp.Or(line.Message.Model, s.model)
		for _, block := range content {
			if fields, ok := block.(map[string]any); ok && fields["type"] == "tool_use" {
				s.tools[strField(fields, "id")] = strField(fields, "name")
			}
		}
		if last := s.last(); last != nil && last.Role == "assistant" && line.Message.ID == *lastID {
			last.Content = append(last.Content, content...)
			return nil
		}
		if s.endsWithToolResult() {
			if err := s.send(ctx, proxy, up, path, rep); err != nil {
				return err
			}
		}
		*lastID = line.Message.ID
		s.messages = append(s.messages, message{Role: "assistant", Content: content})
	default:
		if last := s.last(); last != nil && last.Role == "user" {
			last.Content = append(last.Content, content...)
			return nil
		}
		s.messages = append(s.messages, message{Role: "user", Content: content})
	}
	return nil
}

func (s *session) last() *message {
	if len(s.messages) == 0 {
		return nil
	}
	return &s.messages[len(s.messages)-1]
}

func (s *session) endsWithToolResult() bool {
	last := s.last()
	if last == nil || last.Role != "user" {
		return false
	}
	return slices.ContainsFunc(last.Content, func(block any) bool {
		fields, ok := block.(map[string]any)
		return ok && fields["type"] == "tool_result"
	})
}

// blocks returns message content as a list of blocks.
func blocks(content any) []any {
	switch c := content.(type) {
	case string:
		return []any{map[string]any{"type": "text", "text": c}}
	case []any:
		return slices.Clone(c)
	}
	return nil
}

// send posts the conversation so far through the proxy, then compares what
// it forwarded with the previous request.
func (s *session) send(ctx context.Context, proxy http.Handler, up *upstream, path string, rep *replayReport) error {
	body, err := json.Marshal(map[string]any{"model": cmp.Or(s.model, "claude"), "max_tokens": 1, "messages": s.messages})
	if err != nil {
		return err
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "claude-cli/replay")
	w := httptest.NewRecorder()
	proxy.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return fmt.Errorf("proxy answered %d: %s", w.Code, w.Body.String())
	}
	orig, err := canonical(body)
	if err != nil {
		return err
	}
	fwd, err := canonical(up.body)
	if err != nil {
		return err
	}
	rep.Requests++
	for i := range min(len(s.prevOrig), len(orig), len(fwd)) {
		if orig[i] != s.prevOrig[i] {
			continue
		}
		rep.Compared++
		if fwd[i] != s.prevFwd[i] {
			rep.Unstable++
			if len(rep.Examples) < 5 {
				rep.Examples = append(rep.Examples, fmt.Sprintf("%s: request %d, message %d", filepath.Base(path), rep.Requests, i))
			}
		}
	}
	if s.price.ID == "" {
		var ok bool
		if s.price, ok = cost.Lookup(s.model); !ok {
			s.price, _ = cost.Lookup(cost.AgentModels["claude"])
			rep.Assumed++
		}
	}
	rep.CostRaw += s.promptCost(orig, s.prevOrig)
	rep.CostOut += s.promptCost(fwd, s.prevFwd)
	s.prevOrig, s.prevFwd = orig, fwd
	return s.measure(body, up.body)
}

// promptCost prices a request's messages with prompt caching: the run the
// previous request sent identically is read from the cache, the rest is
// written to it.
func (s *session) promptCost(msgs, prev []string) float64 {
	read, write := 0, 0
	cached := true
	for i, m := range msgs {
		cached = cached && i < len(prev) && m == prev[i]
		n, ok := s.tokens[m]
		if !ok {
			n = tokens.Estimate(m)
			s.tokens[m] = n
		}
		if cached {
			read += n
		} else {
			write += n
		}
	}
	f := cost.Tokenizers[s.price.Tokenizer].Factor
	return f * (float64(read)*s.price.CacheRead + float64(write)*s.price.CacheWrite) / 1e6
}

// canonical re-encodes each message of a request body, so key order and
// escaping don't count as changes.
func canonical(body []byte) ([]string, error) {
	var doc struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := make([]string, len(doc.Messages))
	for i, m := range doc.Messages {
		var v any
		dec := json.NewDecoder(bytes.NewReader(m))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		encoded, err := encodeJSON(v)
		if err != nil {
			return nil, err
		}
		out[i] = string(encoded)
	}
	return out, nil
}

// measure records each tool result's tokens, sent and forwarded.
func (s *session) measure(orig, fwd []byte) error {
	before, err := toolResults(orig)
	if err != nil {
		return err
	}
	after, err := toolResults(fwd)
	if err != nil {
		return err
	}
	for id, text := range before {
		s.results[id] = resultTokens{tokens.Estimate(text), tokens.Estimate(after[id]), strings.Contains(after[id], "[↑ ")}
	}
	return nil
}

// toolResults maps tool_use ids to their result text.
func toolResults(body []byte) (map[string]string, error) {
	var doc struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, m := range doc.Messages {
		for _, b := range blocks(m.Content) {
			r, ok := b.(map[string]any)
			if !ok || r["type"] != "tool_result" {
				continue
			}
			out[strField(r, "tool_use_id")] = toolResultText(r)
		}
	}
	return out, nil
}

// toolResultText joins the text blocks of one tool_result block.
func toolResultText(result map[string]any) string {
	var text []string
	for _, c := range blocks(result["content"]) {
		if t, ok := c.(map[string]any)["text"].(string); ok {
			text = append(text, t)
		}
	}
	return strings.Join(text, "\n")
}
