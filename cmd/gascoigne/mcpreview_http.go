package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

// setLazy saves which tools of one server load on request. It accepts only a
// proxied server and tool names the review already shows for it, so the
// endpoint cannot be made to write anything else into the prefs.
func setLazy(store *ledger.Ledger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var in struct {
			ID    string   `json:"id"`
			Tools []string `json:"tools"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(writer, "want {id, tools}: "+err.Error(), http.StatusBadRequest)
			return
		}
		review, err := loadReview(request.Context(), store, time.Now(), nil, nil)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		i := slices.IndexFunc(review.Servers, func(server mcpServer) bool { return server.ID == in.ID && server.Sized })
		if i < 0 {
			http.Error(writer, "unknown server: only servers started through mcp-proxy can load tools on request", http.StatusBadRequest)
			return
		}
		known := map[string]bool{}
		for _, t := range review.Servers[i].Tools {
			known[t.Name] = !lazyModeTool(t.Name)
		}
		for _, t := range in.Tools {
			if !known[t] {
				http.Error(writer, "unknown tool "+t+" for this server", http.StatusBadRequest)
				return
			}
		}
		p := readPrefs()
		tools := slices.Compact(slices.Sorted(slices.Values(in.Tools)))
		if len(tools) == 0 {
			delete(p.LazyTools, in.ID)
		} else {
			if p.LazyTools == nil {
				p.LazyTools = map[string][]string{}
			}
			p.LazyTools[in.ID] = tools
		}
		if err := p.save(); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		review, err = loadReview(request.Context(), store, time.Now(), p.LazyTools, p.LearnedMCP)
		respond(writer, review, err)
	}
}

func idSet(ids []string) map[string]bool {
	m := make(map[string]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// learnServers saves the servers that are still learning as judged now, all
// of them when id is empty, else only that one. It reports the review as it
// is afterwards. An unknown id is an error; one already judged is a no-op.
func learnServers(ctx context.Context, store *ledger.Ledger, id string) (mcpReview, error) {
	p := readPrefs()
	review, err := loadReview(ctx, store, time.Now(), p.LazyTools, p.LearnedMCP)
	if err != nil {
		return review, err
	}
	if id != "" && !slices.ContainsFunc(review.Servers, func(server mcpServer) bool { return server.ID == id }) {
		return review, fmt.Errorf("unknown server %q", id)
	}
	for _, s := range review.Servers {
		if s.Verdict == verdictLearning && (id == "" || s.ID == id) {
			p.LearnedMCP = append(p.LearnedMCP, s.ID)
		}
	}
	slices.Sort(p.LearnedMCP)
	p.LearnedMCP = slices.Compact(p.LearnedMCP)
	if err := p.save(); err != nil {
		return review, err
	}
	return loadReview(ctx, store, time.Now(), p.LazyTools, p.LearnedMCP)
}

// learnNow is POST /api/mcp/learn {id}: judge one still-learning server now,
// or every one when id is left out.
func learnNow(store *ledger.Ledger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(writer, "want {id}: "+err.Error(), http.StatusBadRequest)
			return
		}
		review, err := learnServers(request.Context(), store, in.ID)
		if err != nil && strings.HasPrefix(err.Error(), "unknown server") {
			http.Error(writer, err.Error(), http.StatusBadRequest)
			return
		}
		respond(writer, review, err)
	}
}

// setRules is POST /api/mcp/rules {id, rules}: it replaces the manual rules of
// one server, an empty list clears them. Like setLazy it takes only a server
// started through mcp-proxy: the others never see a rule.
func setRules(store *ledger.Ledger) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var in struct {
			ID    string                  `json:"id"`
			Rules []mcpproxyreceiver.Rule `json:"rules"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(writer, "want {id, rules}: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(in.Rules) > maxRules {
			http.Error(writer, fmt.Sprintf("at most %d rules per server", maxRules), http.StatusBadRequest)
			return
		}
		for _, rule := range in.Rules {
			if err := rule.Validate(); err != nil {
				http.Error(writer, err.Error(), http.StatusBadRequest)
				return
			}
		}
		p := readPrefs()
		review, err := loadReview(request.Context(), store, time.Now(), p.LazyTools, p.LearnedMCP)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		if i := slices.IndexFunc(review.Servers, func(server mcpServer) bool { return server.ID == in.ID && server.Sized }); i < 0 {
			http.Error(writer, "unknown server: only servers started through mcp-proxy take rules", http.StatusBadRequest)
			return
		}
		if len(in.Rules) == 0 {
			delete(p.MCPRules, in.ID)
		} else {
			if p.MCPRules == nil {
				p.MCPRules = map[string][]mcpproxyreceiver.Rule{}
			}
			p.MCPRules[in.ID] = in.Rules
		}
		if err := p.save(); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		review, err = loadReview(request.Context(), store, time.Now(), p.LazyTools, p.LearnedMCP)
		respond(writer, review, err)
	}
}

// maxRules bounds a server's saved rules: the page offers a handful.
const maxRules = 50
