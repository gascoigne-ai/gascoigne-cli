package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

// keys flattens suggestions to "kind:tool:action" so a table can list them.
func keys(sugs []suggestion) []string {
	out := []string{}
	for _, s := range sugs {
		out = append(out, s.Kind+":"+s.Tool+":"+s.Action)
	}
	return out
}

// tools makes n listed tools of the given size, each called calls times in 7 days.
func tools(n, tokens, calls int) []mcpTool {
	var out []mcpTool
	for i := range n {
		out = append(out, mcpTool{Name: fmt.Sprintf("t%02d", i), Tokens: tokens, Calls7: calls, Calls: calls})
	}
	return out
}

func TestSuggestMCP(t *testing.T) {
	const id = "kube#aaaa1111"
	srv := func(verdict string, tools []mcpTool, mod ...func(*mcpServer)) mcpServer {
		s := mcpServer{ID: id, Name: "kube", Verdict: verdict, Sized: true, Tools: tools}
		for _, m := range mod {
			m(&s)
		}
		return s
	}
	mix := []mcpTool{
		{Name: "get", Tokens: 200, Calls7: 4, Calls: 6},
		{Name: "drain", Tokens: 400},
		{Name: "cordon", Tokens: 300},
		{Name: "tiny", Tokens: 50},
		{Name: "search_tools", Tokens: 150},
		{Name: "call_tool", Tokens: 120},
	}
	tests := []struct {
		name string
		s    mcpServer
		want []string
	}{
		{"idle tools of a used server load on request; called, tiny and lazy-mode tools stay", srv(verdictUsed, mix),
			[]string{"tool:drain:lazy", "tool:cordon:lazy"}},
		{"a tool called once this week is not idle", srv(verdictUsed, tools(3, 250, 1)), []string{}},
		{"idle tokens under the price of lazy mode are not worth it", srv(verdictUsed, []mcpTool{{Name: "a", Tokens: 200, Calls7: 1}, {Name: "b", Tokens: 250}, {Name: "c", Tokens: 200}}), []string{}},
		{"learning: no verdict, no suggestion", srv(verdictLearning, mix), []string{}},
		{"not seen lately: probably removed", srv(verdictStale, mix), []string{}},
		{"a server that stays direct cannot load on request", srv(verdictUsed, mix, func(s *mcpServer) { s.Sized = false }), []string{}},
		{"already lazy and chosen tools are left alone", srv(verdictUsed, mix, func(s *mcpServer) {
			s.Tools = slices.Clone(mix)
			s.Tools[2].Lazy = true
			s.Chosen = []string{"drain"}
		}), []string{}},
		{"choosing part of them leaves the rest suggested", srv(verdictUsed, append(slices.Clone(mix), mcpTool{Name: "taint", Tokens: 300}),
			func(s *mcpServer) { s.Chosen = []string{"drain"} }), []string{"tool:cordon:lazy", "tool:taint:lazy"}},
		{"a tool a rule names is the user's call", srv(verdictUsed, append(slices.Clone(mix), mcpTool{Name: "taint", Tokens: 300}),
			func(s *mcpServer) { s.Rules = []mcpproxyreceiver.Rule{{Match: "drain", Action: "skip"}} }), []string{"tool:cordon:lazy", "tool:taint:lazy"}},
		{"so is one a glob or server/tool rule covers", srv(verdictUsed, append(slices.Clone(mix), mcpTool{Name: "taint", Tokens: 300}, mcpTool{Name: "evict", Tokens: 300}),
			func(s *mcpServer) {
				s.Rules = []mcpproxyreceiver.Rule{{Match: "d*", Action: "shrink"}, {Match: "kube/taint", Action: "skip"}}
			}), []string{"tool:cordon:lazy", "tool:evict:lazy"}},
		{"lazy mode already paid for: a small gain is worth it", srv(verdictUsed, []mcpTool{{Name: "a", Tokens: 200, Calls7: 1}, {Name: "b", Tokens: 150}},
			func(s *mcpServer) { s.Chosen = []string{"gone"} }), []string{"tool:b:lazy"}},
		{"an unused server: its tools load on request and it can go", srv(verdictUnused, tools(4, 250, 0)),
			[]string{"tool:t00:lazy", "tool:t01:lazy", "tool:t02:lazy", "tool:t03:lazy", "server::remove"}},
		{"20 listed tools: max_tools", srv(verdictUsed, tools(20, 100, 1)), []string{"server::max_tools"}},
		{"19 listed tools: not yet", srv(verdictUsed, tools(19, 100, 1)), []string{}},
		{"long definitions: trim_descriptions", srv(verdictUsed, tools(3, 300, 1)), []string{"server::trim_descriptions"}},
		{"just under 300 tokens a tool: not yet", srv(verdictUsed, tools(3, 299, 1)), []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keys(suggestMCP(tt.s)); !slices.Equal(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("a suggestion carries its server, a reason and what it saves", func(t *testing.T) {
		got := suggestMCP(srv(verdictUsed, mix))
		if len(got) != 2 {
			t.Fatalf("got %v", got)
		}
		for _, s := range got {
			if s.Server != id || s.Reason == "" {
				t.Errorf("incomplete %+v", s)
			}
		}
		if got[0].Saves != 400 || got[1].Saves != 300 {
			t.Errorf("saves %d, %d, want 400, 300", got[0].Saves, got[1].Saves)
		}
		if !got[0].applies() || (suggestion{Kind: kindServer, Action: actionRemove}).applies() {
			t.Error("only a tool suggestion is applied by auto-optimize")
		}
	})
}

// On the review's own fixture: github idles 29 tools, linear is unused, sentry
// idles 4, figma is still learning, db stays direct.
func TestBuildReviewSuggests(t *testing.T) {
	stats, cmds, now := reviewFixture()
	r := buildReview(stats, cmds, now, nil)
	got := map[string][]suggestion{}
	for _, s := range r.Servers {
		got[s.Name] = s.Suggestions
	}
	count := func(name, action string) (n int) {
		for _, s := range got[name] {
			if s.Action == action {
				n++
			}
		}
		return
	}
	for _, c := range []struct {
		server, action string
		n              int
	}{
		{"github", actionLazy, 29}, {"github", actionMaxTools, 1}, {"github", actionRemove, 0},
		{"linear", actionLazy, 12}, {"linear", actionRemove, 1},
		{"sentry", actionLazy, 4}, {"sentry", actionRemove, 0},
	} {
		if n := count(c.server, c.action); n != c.n {
			t.Errorf("%s: %d × %s, want %d", c.server, n, c.action, c.n)
		}
	}
	if len(got["figma"]) != 0 || len(got["db"]) != 0 {
		t.Errorf("figma %v, db %v: no suggestions for a learning or a direct server", got["figma"], got["db"])
	}
}

// The default order is by usage: a small, busy server comes before a big idle one.
func TestBuildReviewSortByUsage(t *testing.T) {
	now := time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)
	today := now.Format("2006-01-02")
	row := func(server, tool string, tokens, calls int) ledger.ToolStat {
		return ledger.ToolStat{Day: today, Agent: "claude", Server: server, Tool: tool, Tokens: tokens, Calls: calls}
	}
	stats := []ledger.ToolStat{
		row("big#aaaa1111", "x", 9000, 0),
		row("busy#bbbb2222", "a", 100, 1), row("busy#bbbb2222", "b", 800, 0), row("busy#bbbb2222", "c", 300, 5), row("busy#bbbb2222", "d", 300, 0), row("busy#bbbb2222", "e", 300, 5),
		row("mid#cccc3333", "y", 500, 1),
	}
	r := buildReview(stats, nil, now, nil)
	var order []string
	for _, s := range r.Servers {
		order = append(order, s.Name)
	}
	if want := []string{"busy", "mid", "big"}; !slices.Equal(order, want) {
		t.Fatalf("servers %v, want %v: calls first, then tokens", order, want)
	}
	var names []string
	for _, tl := range r.Servers[0].Tools {
		names = append(names, tl.Name)
	}
	if want := []string{"c", "e", "a", "b", "d"}; !slices.Equal(names, want) {
		t.Fatalf("tools %v, want %v: calls, then tokens, then name", names, want)
	}
}

func TestOptimize(t *testing.T) {
	sandbox(t)
	l, h := testDashboard(t)
	ctx := t.Context()
	const kube, docs = "kube#aaaa1111", "docs#bbbb2222"
	sizes := map[string]map[string]int{
		kube: {"get": 200, "drain": 400, "cordon": 300, "taint": 300, "tiny": 50},
		docs: {"search": 300, "fetch": 300, "index": 300},
	}
	for id, tokens := range sizes {
		if err := l.RecordToolList(ctx, "claude", id, tokens, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := l.RecordToolCall(ctx, "claude", kube, "get"); err != nil {
		t.Fatal(err)
	}
	if err := l.RecordToolCall(ctx, "claude", "direct-db", "query"); err != nil {
		t.Fatal(err)
	}
	p := readPrefs()
	p.LearnedMCP = []string{kube, docs} // judged now: the test cannot wait two days
	if err := p.save(); err != nil {
		t.Fatal(err)
	}
	post := func(body string, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/mcp/optimize", strings.NewReader(body))
		r.Host = "127.0.0.1:7878"
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	applied := func(w *httptest.ResponseRecorder) (list []string, saves int) {
		var out struct {
			mcpReview
			Applied []suggestion `json:"applied"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%v: %s", err, w.Body.String())
		}
		for _, s := range out.Applied {
			list = append(list, s.Server+"/"+s.Tool)
			saves += s.Saves
		}
		return
	}

	t.Run("refuses what it cannot apply", func(t *testing.T) {
		for name, body := range map[string]string{
			"unknown server":         `{"id":"other#00000000"}`,
			"a server with no sizes": `{"id":"direct-db"}`,
			"not json":               `optimize please`,
		} {
			if w := post(body); w.Code != 400 {
				t.Errorf("%s: %d: %s", name, w.Code, w.Body.String())
			}
		}
		if w := post(`{}`, "Origin", "http://evil.example"); w.Code != 403 {
			t.Errorf("another origin: %d", w.Code)
		}
		if len(readPrefs().LazyTools) != 0 || len(readPrefs().MCPRules) != 0 {
			t.Fatalf("prefs written: %+v", readPrefs())
		}
	})

	t.Run("one server: its idle tools load on request, the others are untouched", func(t *testing.T) {
		// Whatever the client sends beyond the id is ignored: the server decides from the ledger.
		w := post(`{"id":"` + kube + `","tools":["rm_rf"]}`)
		if w.Code != 200 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		list, saves := applied(w)
		if want := []string{kube + "/drain", kube + "/cordon", kube + "/taint"}; !slices.Equal(list, want) || saves != 1000 {
			t.Fatalf("applied %v saving %d", list, saves)
		}
		if got := readPrefs().LazyTools[kube]; !slices.Equal(got, []string{"cordon", "drain", "taint"}) {
			t.Fatalf("prefs hold %v", got)
		}
		if _, ok := readPrefs().LazyTools[docs]; ok {
			t.Fatal("another server was touched")
		}
		if got := lazyChoices()[kube]; len(got) != 3 {
			t.Fatalf("mcp-proxy would read %v", got)
		}
		var review mcpReview
		json.Unmarshal(w.Body.Bytes(), &review)
		for _, s := range review.Servers {
			if s.ID == kube && slices.ContainsFunc(s.Suggestions, suggestion.applies) {
				t.Fatalf("still suggested after applying: %v", keys(s.Suggestions))
			}
		}
	})

	t.Run("again: nothing changes", func(t *testing.T) {
		before := readPrefs().LazyTools[kube]
		w := post(`{"id":"` + kube + `"}`)
		if list, _ := applied(w); w.Code != 200 || len(list) != 0 {
			t.Fatalf("%d: applied %v", w.Code, list)
		}
		if !slices.Equal(readPrefs().LazyTools[kube], before) {
			t.Fatal("prefs changed")
		}
	})

	t.Run("no id: every server, keeping an earlier choice", func(t *testing.T) {
		p := readPrefs()
		p.LazyTools[kube] = []string{"taint"}
		p.MCPRules = map[string][]mcpproxyreceiver.Rule{kube: {{Match: "drain", Action: "skip"}}} // the user's rule wins
		if err := p.save(); err != nil {
			t.Fatal(err)
		}
		w := post(`{}`)
		if w.Code != 200 {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
		if got := readPrefs().LazyTools[kube]; !slices.Equal(got, []string{"cordon", "taint"}) {
			t.Fatalf("kube holds %v: taint kept, cordon added, drain left to its rule", got)
		}
		if got := readPrefs().LazyTools[docs]; !slices.Equal(got, []string{"fetch", "index", "search"}) {
			t.Fatalf("docs holds %v", got)
		}
		if got := readPrefs().MCPRules[kube]; len(got) != 1 {
			t.Fatalf("the user's rules were changed: %v", got)
		}
	})
}

// The page offers what the endpoint does, and sorts a server's tools too.
func TestMCPPageOptimizes(t *testing.T) {
	page := string(mcpHTML)
	for _, want := range []string{"Auto-optimize", "/api/mcp/optimize", "suggestions", "aria-sort"} {
		if !strings.Contains(page, want) {
			t.Errorf("mcp page lacks %q", want)
		}
	}
	if !strings.Contains(page, "toolColumns") {
		t.Error("the tool table does not sort")
	}
	if strings.Contains(page, "innerHTML") {
		t.Error("the page builds DOM from strings")
	}
}

// The terminal form lists the suggestions: the tools to load on request in one
// line per server, the advice apart.
func TestPrintReviewSuggestions(t *testing.T) {
	stats, cmds, now := reviewFixture()
	var b strings.Builder
	printReview(&b, buildReview(stats, cmds, now, nil))
	out := b.String()
	for _, want := range []string{"SUGGESTIONS", "linear: load on request", "linear: nothing called it", "github: 30 tools are listed", "-optimize"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "figma: load") || strings.Contains(out, "db: load") {
		t.Errorf("a learning or direct server is suggested:\n%s", out)
	}
}
