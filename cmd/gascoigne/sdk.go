package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// sdkProto is the version of the /sdk/v1 wire contract that gascoigne-sdk
// speaks. Additive changes keep it; a breaking one bumps it and the path.
const sdkProto = 1

// sdkPipeline optimizes SDK payloads: the same filters and optimizers as
// `gascoigne run`.
const sdkPipeline = "cli"

type sdkRequest struct {
	Text    string `json:"text"`
	Tool    string `json:"tool,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	URL     string `json:"url,omitempty"`
	Query   string `json:"query,omitempty"`
	Session string `json:"session,omitempty"`
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
}

type sdkResponse struct {
	Text      string `json:"text"`
	RawID     string `json:"raw_id,omitempty"`
	RawTokens int    `json:"raw_tokens"`
	OutTokens int    `json:"out_tokens"`
	Skipped   string `json:"skipped,omitempty"`
}

type sdkHealth struct {
	Version string `json:"version"`
	Proto   int    `json:"proto"`
	Proxy   bool   `json:"proxy"`
}

// attrs maps the request's set fields to payload attributes.
func (r sdkRequest) attrs() map[string]string {
	attrs := map[string]string{}
	for k, v := range map[string]string{
		payload.AttrTool: r.Tool, payload.AttrCmd: r.Cmd, payload.AttrURL: r.URL,
		payload.AttrQuery: r.Query, payload.AttrSession: r.Session,
		payload.AttrAgent: r.Agent, payload.AttrModel: r.Model,
	} {
		if v != "" {
			attrs[k] = v
		}
	}
	return attrs
}

// sdkOptimize serves POST /sdk/v1/optimize: the text of one tool result in,
// the optimized text out, through the user's own config and ledger.
func sdkOptimize(w *warm) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		var req sdkRequest
		if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 64<<20)).Decode(&req); err != nil {
			http.Error(rw, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
			return
		}
		data, err := configData("")
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		p := payload.New(payload.ToolResult, req.Text)
		for k, v := range req.attrs() {
			p.Attrs[k] = v
		}
		out, status, err := w.optimize(r.Context(), optimizeRequest{
			Config: string(data), Overrides: config.Overrides{}, Pipeline: sdkPipeline, Payload: p,
		})
		if err != nil {
			http.Error(rw, err.Error(), status)
			return
		}
		resp := sdkResponse{
			Text: out.Text(), RawID: out.Attrs[payload.AttrRawID], Skipped: out.Attrs[payload.AttrSkip],
		}
		fmt.Sscan(out.Attrs[payload.AttrRawTokens], &resp.RawTokens)
		fmt.Sscan(out.Attrs[payload.AttrOutTokens], &resp.OutTokens)
		respond(rw, resp, nil)
	})
}

// sdkHealthHandler serves GET /sdk/v1/health: what an SDK checks before it
// trusts the server.
func sdkHealthHandler(version string, proxy bool) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		respond(rw, sdkHealth{Version: version, Proto: sdkProto, Proxy: proxy}, nil)
	})
}

// sdkRaw serves GET /sdk/v1/raw/{id}: the original behind an optimized
// result. Instruction text is never served, as in the dashboard.
func sdkRaw(l *ledger.Ledger) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		key, p, err := l.GetRaw(r.Context(), r.PathValue("id"))
		if err != nil || hiddenKey(key) {
			http.Error(rw, "original not found (expired or unknown id)", http.StatusNotFound)
			return
		}
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		io.WriteString(rw, p.Text())
	})
}
