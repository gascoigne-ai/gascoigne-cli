package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// reviewServer answers GET /api/mcp like the background server, with the
// review in r, after delay.
func reviewServer(t *testing.T, r mcpReview, delay time.Duration) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		time.Sleep(delay)
		json.NewEncoder(w).Encode(r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GASCOIGNE_SERVER_ADDR", strings.TrimPrefix(srv.URL, "http://"))
}

func nudgeEnv(t *testing.T) {
	t.Helper()
	sandbox(t)
	t.Setenv("CI", "true") // never open a real browser from a test
	t.Setenv("GASCOIGNE_SERVER", "")
	t.Setenv("GASCOIGNE_MCP_REVIEW", "")
}

func tool(name string, tokens, calls int) mcpTool {
	return mcpTool{Name: name, Tokens: tokens, Calls: calls, Calls7: calls}
}

// unusedServer: proxied, seen for two weeks, nobody called it, 2,000 tokens per request.
var unusedServer = mcpServer{ID: "linear#aaaa1111", Name: "linear", Label: "linear", Verdict: verdictUnused, Sized: true, Tokens: 2000, Seen: 14,
	Tools: []mcpTool{tool("a", 1200, 0), tool("b", 800, 0)}, Chosen: []string{}}

func TestMCPNudge(t *testing.T) {
	t.Run("an unused server opens the review, once a week", func(t *testing.T) {
		nudgeEnv(t)
		reviewServer(t, mcpReview{Servers: []mcpServer{unusedServer}}, 0)
		note := mcpNudge("startup", false)
		if !strings.Contains(note, "2000 tokens") || !strings.Contains(note, "linear") || !strings.Contains(note, "/mcp") {
			t.Fatalf("note %q", note)
		}
		if readPrefs().LastMCPReview.IsZero() {
			t.Fatal("the week was not recorded")
		}
		if again := mcpNudge("resume", false); again != "" {
			t.Fatalf("a second nudge within the week: %q", again)
		}
		p := readPrefs()
		p.LastMCPReview = time.Now().Add(-8 * 24 * time.Hour)
		p.save()
		if next := mcpNudge("startup", false); next == "" {
			t.Fatal("a week later it stays quiet")
		}
	})
	t.Run("idle tools of a used server count until chosen", func(t *testing.T) {
		nudgeEnv(t)
		used := mcpServer{ID: "gh#bbbb2222", Name: "gh", Label: "gh", Verdict: verdictUsed, Sized: true, Tokens: 4000, Seen: 14,
			Tools: []mcpTool{tool("search", 500, 9), tool("x", 1000, 0), tool("y", 1000, 0), tool("search_tools", 150, 0)}, Chosen: []string{}}
		reviewServer(t, mcpReview{Servers: []mcpServer{used}}, 0)
		if note := mcpNudge("startup", false); !strings.Contains(note, "2000 tokens") {
			t.Fatalf("note %q: x and y are idle, the lazy-mode tool is not", note)
		}
		nudgeEnv(t)
		used.Chosen = []string{"x", "y"}
		reviewServer(t, mcpReview{Servers: []mcpServer{used}}, 0)
		if note := mcpNudge("startup", false); note != "" {
			t.Fatalf("already chosen, still nudged: %q", note)
		}
	})
	quiet := map[string]mcpReview{
		"too small":      {Servers: []mcpServer{{ID: "s", Label: "s", Verdict: verdictUnused, Sized: true, Tokens: 900, Tools: []mcpTool{tool("a", 900, 0)}}}},
		"still learning": {Servers: []mcpServer{{ID: "s", Label: "s", Verdict: verdictLearning, Sized: true, Tokens: 5000, Tools: []mcpTool{tool("a", 5000, 0)}}}},
		"direct server":  {Servers: []mcpServer{{ID: "db", Label: "db", Verdict: verdictUsed, Sized: false}}},
		"nothing":        {Servers: []mcpServer{}},
	}
	for name, r := range quiet {
		t.Run("quiet: "+name, func(t *testing.T) {
			nudgeEnv(t)
			reviewServer(t, r, 0)
			if note := mcpNudge("startup", false); note != "" {
				t.Fatalf("note %q", note)
			}
			if !readPrefs().LastMCPReview.IsZero() {
				t.Fatal("the week was used up with nothing said")
			}
		})
	}
	t.Run("another page opened first: wait for the next start", func(t *testing.T) {
		nudgeEnv(t)
		reviewServer(t, mcpReview{Servers: []mcpServer{unusedServer}}, 0)
		if note := mcpNudge("startup", true); note != "" {
			t.Fatalf("two pages in one start: %q", note)
		}
		if !readPrefs().LastMCPReview.IsZero() {
			t.Fatal("the week was used up")
		}
	})
	for _, source := range []string{"compact", "clear"} {
		t.Run("not a new session: "+source, func(t *testing.T) {
			nudgeEnv(t)
			reviewServer(t, mcpReview{Servers: []mcpServer{unusedServer}}, 0)
			if note := mcpNudge(source, false); note != "" {
				t.Fatalf("note %q", note)
			}
		})
	}
	t.Run("opt-outs", func(t *testing.T) {
		for name, off := range map[string]func(t *testing.T){
			"env":    func(t *testing.T) { t.Setenv("GASCOIGNE_MCP_REVIEW", "off") },
			"prefs":  func(t *testing.T) { p := readPrefs(); p.MCPReview = "off"; p.save() },
			"server": func(t *testing.T) { t.Setenv("GASCOIGNE_SERVER", "off") },
		} {
			t.Run(name, func(t *testing.T) {
				nudgeEnv(t)
				reviewServer(t, mcpReview{Servers: []mcpServer{unusedServer}}, 0)
				off(t)
				if note := mcpNudge("startup", false); note != "" {
					t.Fatalf("note %q", note)
				}
			})
		}
	})
	t.Run("a server that is down or slow never holds the start", func(t *testing.T) {
		nudgeEnv(t)
		t.Setenv("GASCOIGNE_SERVER_ADDR", "127.0.0.1:1") // nothing listens
		if note := mcpNudge("startup", false); note != "" {
			t.Fatalf("note %q", note)
		}
		reviewServer(t, mcpReview{Servers: []mcpServer{unusedServer}}, 3*time.Second)
		start := time.Now()
		if note := mcpNudge("startup", false); note != "" || time.Since(start) > 2*time.Second {
			t.Fatalf("slow server: note %q after %v", note, time.Since(start))
		}
	})
	t.Run("many servers are summarized", func(t *testing.T) {
		nudgeEnv(t)
		var servers []mcpServer
		for _, n := range []string{"a", "b", "c", "d", "e"} {
			s := unusedServer
			s.ID, s.Label = n+"#00000000", n
			servers = append(servers, s)
		}
		reviewServer(t, mcpReview{Servers: servers}, 0)
		if note := mcpNudge("startup", false); !strings.Contains(note, "and 2 more") {
			t.Fatalf("note %q", note)
		}
	})
}

func TestMCPReviewQuestionIsOptionalAndOnByDefault(t *testing.T) {
	sandbox(t)
	d := mcpReviewDecision(prefs{})
	if !d.Optional || d.Current != "on" || d.Group != "updates" || !strings.Contains(d.Options[0].Detail, "nothing leaves it") {
		t.Fatalf("%+v", d)
	}
	for _, p := range pendingDecisions(prefs{}) {
		if p.ID == "mcp_review" {
			t.Error("the weekly review never counts as an open question")
		}
	}
	if got := mcpReviewDecision(prefs{MCPReview: "off"}); got.Current != "off" {
		t.Errorf("saved off: %+v", got)
	}
	if _, err := applyAnswers(map[string]string{"mcp_review": "off"}); err != nil || readPrefs().MCPReview != "off" {
		t.Fatalf("answer not saved: %v %+v", err, readPrefs())
	}
}
