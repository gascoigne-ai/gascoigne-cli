// Package httpproxyreceiver sits between a coding agent and its LLM API
// (ANTHROPIC_BASE_URL, an OpenAI base URL) and shortens the tool results in
// each request before forwarding it, so what the agent's own tools return
// (Claude Code's Read above all) is optimized too. Everything else passes
// through: system prompt, tools, user and assistant text, headers
// (credentials included) and the streamed response.
//
// Every request resends the whole conversation, and the provider caches its
// prefix. Each tool result is therefore transformed once and remembered by
// content hash, so later requests send exactly the same bytes; optimizers
// are deterministic, so a restart recomputes them identically.
package httpproxyreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/mcptool"
)

// RawHeader set to 1 forwards a request untouched.
const RawHeader = "X-Gascoigne-Raw"

type Receiver struct {
	component.Nop
	cfg    *Config
	next   component.Pipeline
	log    *slog.Logger
	cache  *cache
	ledger component.Ledger // keeps originals of folded results; nil: no folding
	active atomic.Int64
}

// New builds the receiver; next optimizes each tool result.
func New(cfg *Config, next component.Pipeline, log *slog.Logger) *Receiver {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Receiver{cfg: cfg, next: next, log: log, cache: newCache(cfg.CacheEntries)}
}

// Start finds the ledger that keeps originals of folded results.
func (r *Receiver) Start(_ context.Context, host component.Host) error {
	if r.cfg.Ledger == "" {
		return nil
	}
	l, err := mcptool.Ledger(host, r.cfg.Ledger)
	if err != nil {
		return fmt.Errorf("httpproxy ledger: %w", err)
	}
	r.ledger = l
	return nil
}

// Run does nothing: the receiver is an http.Handler, mounted by its host
// (gascoigne's background server serves it under /v1/).
func (r *Receiver) Run(context.Context) error { return nil }

// Active is the number of requests being forwarded; a host restarting
// itself waits for zero so no agent stream is cut.
func (r *Receiver) Active() int64 { return r.active.Load() }

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.active.Add(1)
	defer r.active.Add(-1)
	api, upstream := r.route(req)
	raw := req.Header.Get(RawHeader) == "1"
	req.Header.Del(RawHeader)
	if req.Method == http.MethodPost && !raw && api != nil {
		r.rewrite(req, api)
	}
	target, err := url.Parse(upstream)
	if err != nil {
		http.Error(w, "bad upstream", http.StatusBadGateway)
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
		},
		FlushInterval: -1, // stream server-sent events as they come
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			r.log.Warn("upstream", "err", err)
			http.Error(w, "gascoigne proxy: upstream: "+err.Error(), http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, req)
}

// rewriter shortens the tool results of one API's request body.
type rewriter func(ctx context.Context, r *Receiver, body map[string]any, meta meta) bool

// route picks the API format and upstream: Anthropic requests carry
// anthropic-version (or x-api-key), everything else is OpenAI-style.
func (r *Receiver) route(req *http.Request) (rewriter, string) {
	p := req.URL.Path
	if req.Header.Get("anthropic-version") != "" || req.Header.Get("x-api-key") != "" || strings.HasPrefix(p, "/v1/messages") {
		if strings.HasPrefix(p, "/v1/messages") {
			return anthropic, r.cfg.Anthropic
		}
		return nil, r.cfg.Anthropic
	}
	switch {
	case strings.HasSuffix(p, "/chat/completions"):
		return chatCompletions, r.cfg.OpenAI
	case strings.HasSuffix(p, "/responses"):
		return responses, r.cfg.OpenAI
	}
	return nil, r.cfg.OpenAI
}

// meta is what a request says about where its tool results go.
type meta struct {
	model  string
	agent  string
	folder *folder // results seen so far in this request
}

// rewrite replaces req's body with shortened tool results; on any problem
// the original body is forwarded.
func (r *Receiver) rewrite(req *http.Request, api rewriter) {
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || !strings.Contains(req.Header.Get("Content-Type"), "json") {
		return
	}
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // integers stay exact
	if dec.Decode(&doc) != nil {
		return
	}
	model, _ := doc["model"].(string)
	m := meta{model: model, agent: agentOf(req.Header.Get("User-Agent"))}
	if r.cfg.FoldMinLines > 0 {
		m.folder = newFolder(r.cfg.FoldMinLines, r.cfg.FoldMinChars)
	}
	if !api(req.Context(), r, doc, m) {
		return
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if enc.Encode(doc) != nil {
		return
	}
	b := bytes.TrimSuffix(out.Bytes(), []byte("\n"))
	req.Body = io.NopCloser(bytes.NewReader(b))
	req.ContentLength = int64(len(b))
	req.Header.Del("Content-Length")
}

// agentOf names the agent from its User-Agent ("claude-cli/2.1 …").
func agentOf(ua string) string {
	ua = strings.ToLower(ua)
	for _, a := range []struct{ marker, name string }{
		{"claude-cli", "claude"}, {"claude-code", "claude"}, {"codex", "codex"}, {"opencode", "opencode"},
		{"gemini", "gemini"}, {"cursor", "cursor"}, {"aider", "aider"},
	} {
		if strings.Contains(ua, a.marker) {
			return a.name
		}
	}
	return ""
}
