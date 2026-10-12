package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

const (
	// mcpReviewInterval is how often a session start may open the review page.
	mcpReviewInterval = 7 * 24 * time.Hour
	// mcpNudgeTokens is how many tokens per request of unused tool definitions
	// make a page worth opening.
	mcpNudgeTokens = 1500
	// mcpNudgeTimeout bounds the question to the background server: a hook
	// must never make a session start wait.
	mcpNudgeTimeout = time.Second
)

// mcpReviewOn: the weekly review is on unless the user or the environment says off.
func mcpReviewOn() bool {
	return os.Getenv("GASCOIGNE_MCP_REVIEW") != "off" && readPrefs().MCPReview != "off"
}

// mcpNudge is what a session start does once a week when MCP tool definitions
// that nobody used are listed to the agent: open the review page and tell
// the agent. A compaction or /clear is not a new session, and a page another
// notice just opened comes first (one page per session start; the week is
// not used up, so the next start tries again).
func mcpNudge(source string, pageOpened bool) string {
	if pageOpened || source == "compact" || source == "clear" || serverOff() || !mcpReviewOn() {
		return ""
	}
	p := readPrefs()
	if time.Since(p.LastMCPReview) < mcpReviewInterval {
		return ""
	}
	review, err := fetchReview()
	if err != nil {
		return ""
	}
	tokens, labels := reviewWorth(review)
	if tokens < mcpNudgeTokens {
		return ""
	}
	p.LastMCPReview = time.Now()
	if p.save() != nil {
		return ""
	}
	link := "http://" + serverAddr() + "/mcp"
	if os.Getenv("CI") == "" {
		openBrowser(link)
	}
	shown := strings.Join(labels[:min(len(labels), 3)], ", ")
	if len(labels) > 3 {
		shown += fmt.Sprintf(" and %d more", len(labels)-3)
	}
	return fmt.Sprintf("gascoigne: about %d tokens of MCP tool definitions (%s) were not used in the last 14 days and are listed to the agent. Review them in the browser (%s), or run gascoigne mcp-review. Nothing changes without the user's choice.", tokens, shown, link)
}

// fetchReview asks the running background server for the review; it never
// starts one.
func fetchReview() (mcpReview, error) {
	var r mcpReview
	client := http.Client{Timeout: mcpNudgeTimeout}
	resp, err := client.Get("http://" + serverAddr() + "/api/mcp")
	if err != nil {
		return r, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return r, fmt.Errorf("mcp review: %s", resp.Status)
	}
	return r, json.NewDecoder(http.MaxBytesReader(nil, resp.Body, 4<<20)).Decode(&r)
}

// reviewWorth adds up what the user can act on: the definitions of unused
// servers and of idle tools not yet chosen to load on request, in tokens per
// request, and the servers they belong to. Servers without sizes (direct) and
// servers seen for less than a week and servers not listed lately (probably removed) are left out.
func reviewWorth(r mcpReview) (tokens int, labels []string) {
	for _, s := range r.Servers {
		if !s.Sized || s.Verdict == verdictLearning || s.Verdict == verdictStale {
			continue
		}
		n := s.idleUnchosenTokens()
		if s.Verdict == verdictUnused {
			n = s.Tokens
		}
		if n > 0 {
			tokens += n
			labels = append(labels, s.Label)
		}
	}
	return tokens, labels
}

// idleUnchosenTokens: definitions of tools not called in unusedDays that still
// are listed to the agent (not lazy, not chosen to be).
func (s mcpServer) idleUnchosenTokens() int {
	n := 0
	for _, t := range s.Tools {
		if !t.Lazy && t.Calls == 0 && !lazyModeTool(t.Name) && !slices.Contains(s.Chosen, t.Name) {
			n += t.Tokens
		}
	}
	return n
}

// mcpReviewDecision is the setup question about the weekly page; optional,
// on until the user says otherwise.
func mcpReviewDecision(p prefs) decision {
	current := "on"
	if p.MCPReview == "off" {
		current = "off"
	}
	return decision{ID: "mcp_review", Group: "updates", Optional: true, Current: current, Default: "on",
		Title: "Review my MCP servers once a week",
		Text:  "MCP servers add their tool definitions to the agent's tool list, used or not; most agents send that list with every request.",
		Options: []option{
			{"on", "Open the review when it finds something", "At most once a week, at a session start, and only when tools nobody called in 14 days cost over " + fmt.Sprint(mcpNudgeTokens) + " tokens per request. It opens a page on this machine; nothing leaves it, and nothing changes unless you choose something there."},
			{"off", "Never open it", "You can still run gascoigne mcp-review whenever you like."},
		}}
}
