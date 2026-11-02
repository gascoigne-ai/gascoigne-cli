package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

// Suggestions of the MCP review: what the usage patterns say to change. Tool
// suggestions are applied by auto-optimize (loading a tool on request loses
// nothing); server suggestions are advice, because they live in the agent's
// own configuration, which gascoigne does not write.

// Kinds and actions of a suggestion.
const (
	kindTool   = "tool"
	kindServer = "server"

	actionLazy             = mcpproxyreceiver.ActionLazy
	actionRemove           = "remove"
	actionMaxTools         = "max_tools"
	actionTrimDescriptions = "trim_descriptions"
)

// Thresholds of the suggestions.
const (
	lazyMinTool   = 100 // a smaller definition is not worth a lookup
	lazyMinSaving = 500 // lazy mode adds search_tools and call_tool (about 300 tokens): idle tokens must clearly beat that
	maxToolsMin   = 20  // listed tools from which a cap on the list pays
	trimAvgTokens = 300 // average definition size from which shorter descriptions pay
)

// suggestion is one change the usage patterns support.
type suggestion struct {
	Server string `json:"server"`
	Tool   string `json:"tool,omitempty"`
	Kind   string `json:"kind"`
	Action string `json:"action"`
	Reason string `json:"reason"`
	Saves  int    `json:"saves"` // tokens out of the tool list; 0 for advice that cannot be sized
}

// applies reports whether auto-optimize does it.
func (s suggestion) applies() bool { return s.Kind == kindTool }

// serverAdvice are the suggestions about a whole server, tried in order. listed
// are its tools still in the tool list.
var serverAdvice = []struct {
	action string
	when   func(s mcpServer, listed []mcpTool) (reason string, ok bool)
}{
	{actionRemove, func(s mcpServer, _ []mcpTool) (string, bool) {
		return fmt.Sprintf("nothing called it in %d day(s): switch it off", min(s.Seen, unusedDays)), s.Verdict == verdictUnused
	}},
	{actionMaxTools, func(_ mcpServer, listed []mcpTool) (string, bool) {
		return fmt.Sprintf("%d tools are listed: set mcp_proxy.max_tools to list the most used and search the rest", len(listed)), len(listed) >= maxToolsMin
	}},
	{actionTrimDescriptions, func(_ mcpServer, listed []mcpTool) (string, bool) {
		sum := 0
		for _, t := range listed {
			sum += t.Tokens
		}
		avg := sum / max(len(listed), 1)
		return fmt.Sprintf("definitions average %d tokens: set mcp_proxy.trim_descriptions", avg), len(listed) > 0 && avg >= trimAvgTokens
	}},
}

// suggestMCP is what the usage of one server supports. A server that is still
// learning, was not seen lately or stays direct gets none: the data does not
// show anything yet, or nothing can be changed.
func suggestMCP(s mcpServer) []suggestion {
	out := []suggestion{}
	if !s.Sized || s.Verdict == verdictLearning || s.Verdict == verdictStale {
		return out
	}
	chosen := map[string]bool{}
	for _, n := range s.Chosen {
		chosen[n] = true
	}
	var idle, listed []mcpTool
	paid := len(chosen) > 0 // lazy mode's own two tools are then in the list already
	for _, t := range s.Tools {
		switch {
		case lazyModeTool(t.Name):
			continue
		case t.Lazy:
			paid = true
			continue
		case chosen[t.Name]:
			continue
		}
		listed = append(listed, t)
		if t.Calls7 == 0 && t.Tokens >= lazyMinTool && mcpproxyreceiver.Resolve(s.Rules, s.ID, t.Name) == "" { // a rule is the user's call
			idle = append(idle, t)
		}
	}
	saving := 0
	for _, t := range idle {
		saving += t.Tokens
	}
	if paid || saving >= lazyMinSaving {
		for _, t := range idle {
			out = append(out, suggestion{Server: s.ID, Tool: t.Name, Kind: kindTool, Action: actionLazy, Saves: t.Tokens,
				Reason: fmt.Sprintf("not called in %d day(s): load on request", min(s.Seen, unusedDays))})
		}
	}
	for _, a := range serverAdvice {
		if reason, ok := a.when(s, listed); ok {
			out = append(out, suggestion{Server: s.ID, Kind: kindServer, Action: a.action, Reason: reason})
		}
	}
	return out
}

var errUnknownServer = errors.New("unknown server")

// optimizeServers applies the tool suggestions of one server, of every server
// when id is empty, and reports what it applied. It decides from the ledger
// alone and adds to the tools already chosen, never removes one. Rules are
// not touched.
func optimizeServers(ctx context.Context, l *ledger.Ledger, id string) (mcpReview, []suggestion, error) {
	p := readPrefs()
	review, err := loadReview(ctx, l, time.Now(), p.LazyTools, p.LearnedMCP)
	if err != nil {
		return review, nil, err
	}
	if id != "" && !slices.ContainsFunc(review.Servers, func(s mcpServer) bool { return s.ID == id && s.Sized }) {
		return review, nil, fmt.Errorf("%w %q: only servers started through mcp-proxy can be optimized", errUnknownServer, id)
	}
	applied := []suggestion{}
	for _, s := range review.Servers {
		if id != "" && s.ID != id {
			continue
		}
		tools := slices.Clone(s.Chosen)
		n := len(tools)
		for _, sg := range s.Suggestions {
			if sg.applies() {
				tools = append(tools, sg.Tool)
				applied = append(applied, sg)
			}
		}
		if len(tools) == n {
			continue
		}
		if p.LazyTools == nil {
			p.LazyTools = map[string][]string{}
		}
		p.LazyTools[s.ID] = slices.Compact(slices.Sorted(slices.Values(tools)))
	}
	if len(applied) == 0 {
		return review, applied, nil
	}
	if err := p.save(); err != nil {
		return review, nil, err
	}
	review, err = loadReview(ctx, l, time.Now(), p.LazyTools, p.LearnedMCP)
	return review, applied, err
}

// optimizeMCP is POST /api/mcp/optimize {id}: apply the suggestions of one
// server, or of every one when id is left out.
func optimizeMCP(l *ledger.Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			http.Error(w, "want {id}: "+err.Error(), http.StatusBadRequest)
			return
		}
		review, applied, err := optimizeServers(r.Context(), l, in.ID)
		if errors.Is(err, errUnknownServer) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		respond(w, struct {
			mcpReview
			Applied []suggestion `json:"applied"`
		}{review, applied}, err)
	}
}
