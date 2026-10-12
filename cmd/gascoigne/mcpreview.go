package main

import (
	"cmp"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// The weekly MCP review: which MCP servers cost tokens in every request and
// which of them earn it. It reads two things the ledger keeps for two weeks:
// the table mcp-proxy fills (definition sizes and calls) and the hook's runs
// named mcp__server__tool (calls to servers that stay direct, no sizes).

const (
	// observeDays is how long a server must have been seen before the review
	// calls it unused: a week of silence on a server seen for a day says nothing.
	observeDays = 7
	// unusedDays is the silence that makes a server unused.
	unusedDays = 14
	// rarelyCalls and rarelyTokens: a server with at most this many calls in
	// a week whose definitions cost at least this much in every request.
	rarelyCalls  = 2
	rarelyTokens = 1500
)

// Verdicts of a server.
const (
	verdictLearning = "learning" // seen for less than observeDays
	verdictUnused   = "unused"
	verdictRarely   = "rarely used"
	verdictUsed     = "used"
)

// mcpTool is one tool of a server in the review.
type mcpTool struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"` // definition size, 0 when unknown
	Lazy   bool   `json:"lazy"`   // offered on request only: Tokens is what listing it would cost
	Calls7 int    `json:"calls_7d"`
	Calls  int    `json:"calls_14d"`
}

// mcpServer is one server in the review.
type mcpServer struct {
	ID        string    `json:"id"`    // name#hash of the command line for proxied servers: the key of its stats
	Name      string    `json:"name"`  // what the server reports (a proxied one) or the agent's key for it (a direct one)
	Label     string    `json:"label"` // Name, with the id's hash when two servers share it
	Verdict   string    `json:"verdict"`
	Sized     bool      `json:"sized"`       // false: only calls are known (the server stays direct)
	Tokens    int       `json:"tokens"`      // what its definitions cost in every request
	IdleToks  int       `json:"idle_tokens"` // of those, tools not called in unusedDays
	LazyToks  int       `json:"lazy_tokens"` // what the tools offered on request only would cost if listed
	Calls7    int       `json:"calls_7d"`
	CallsPrev int       `json:"calls_prev_7d"` // the 7 days before
	LastUsed  string    `json:"last_used,omitempty"`
	Seen      int       `json:"seen_days"`
	Agents    []string  `json:"agents"`
	Tools     []mcpTool `json:"tools"`
}

// mcpReview is the whole review.
type mcpReview struct {
	Days    int         `json:"days"` // days of data
	Servers []mcpServer `json:"servers"`
	Notes   []string    `json:"notes"`
}

// buildReview turns the ledger's rows into the review as of now. Days are UTC.
func buildReview(stats []ledger.ToolStat, cmds []ledger.CommandDay, now time.Time) mcpReview {
	today := now.UTC().Truncate(24 * time.Hour)
	age := func(day string) int { // days ago, 0 for today
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			return 1 << 20
		}
		return int(today.Sub(d).Hours() / 24)
	}
	type key struct{ server, tool string }
	servers := map[string]*mcpServer{}
	tools := map[key]*mcpTool{}
	oldest := map[string]int{}               // server -> age of its first row
	latest := map[key]string{}               // tool -> day of the size kept
	listedBy := map[string]map[string]bool{} // proxied server -> the tools it lists
	var dataOldest int

	touch := func(server, agent, day string) *mcpServer {
		s := servers[server]
		if s == nil {
			name, _, _ := strings.Cut(server, "#")
			s = &mcpServer{ID: server, Name: name, Label: name, Agents: []string{}, Tools: []mcpTool{}}
			servers[server] = s
		}
		if agent != "" && !slices.Contains(s.Agents, agent) {
			s.Agents = append(s.Agents, agent)
		}
		a := age(day)
		oldest[server] = max(oldest[server], a)
		dataOldest = max(dataOldest, a)
		return s
	}
	tool := func(server, name string) *mcpTool {
		t := tools[key{server, name}]
		if t == nil {
			t = &mcpTool{Name: name}
			tools[key{server, name}] = t
		}
		return t
	}
	count := func(s *mcpServer, t *mcpTool, day string, n int) {
		a := age(day)
		switch {
		case a < 7:
			s.Calls7 += n
			t.Calls7 += n
		case a < 14:
			s.CallsPrev += n
		}
		if a < unusedDays {
			t.Calls += n
		}
		if n > 0 && (s.LastUsed == "" || day > s.LastUsed) {
			s.LastUsed = day
		}
	}

	for _, r := range stats {
		s := touch(r.Server, r.Agent, r.Day)
		t := tool(r.Server, r.Tool)
		if r.Tokens > 0 {
			k := key{r.Server, r.Tool}
			if r.Day >= latest[k] { // the last listing's size
				t.Tokens, t.Lazy, latest[k] = r.Tokens, r.Lazy, r.Day
			}
			s.Sized = true
		}
		if listedBy[r.Server] == nil {
			listedBy[r.Server] = map[string]bool{}
		}
		listedBy[r.Server][r.Tool] = true
		count(s, t, r.Day, r.Calls)
	}
	covered := coveredByProxy(cmds, listedBy)
	for _, c := range cmds {
		server, name, ok := splitMCPName(c.Command)
		if !ok || covered[server] {
			continue
		}
		s := touch(server, c.Agent, c.Day)
		count(s, tool(server, name), c.Day, c.Runs)
	}

	for k, t := range tools {
		s := servers[k.server]
		s.Tools = append(s.Tools, *t)
	}
	review := mcpReview{Days: dataOldest + 1, Servers: []mcpServer{}}
	for _, s := range servers {
		slices.SortFunc(s.Tools, func(a, b mcpTool) int { return cmp.Or(cmp.Compare(b.Tokens, a.Tokens), cmp.Compare(a.Name, b.Name)) })
		slices.Sort(s.Agents)
		s.Seen = oldest[s.ID] + 1
		for _, t := range s.Tools {
			if t.Lazy {
				s.LazyToks += t.Tokens
				continue
			}
			s.Tokens += t.Tokens
			if t.Calls == 0 && !lazyModeTool(t.Name) { // the lazy-mode tools are the price of lazy tools, never idle
				s.IdleToks += t.Tokens
			}
		}
		s.Verdict = verdict(s)
		review.Servers = append(review.Servers, *s)
	}
	labelDuplicates(review.Servers)
	slices.SortFunc(review.Servers, func(a, b mcpServer) int {
		return cmp.Or(cmp.Compare(b.Tokens, a.Tokens), cmp.Compare(b.Calls7, a.Calls7), cmp.Compare(a.Name, b.Name))
	})
	review.Notes = reviewNotes(review)
	return review
}

func verdict(s *mcpServer) string {
	calls := s.Calls7 + s.CallsPrev
	switch {
	case s.Seen < observeDays:
		return verdictLearning
	case calls == 0:
		return verdictUnused
	case s.Sized && s.Calls7 <= rarelyCalls && s.Tokens >= rarelyTokens:
		return verdictRarely
	}
	return verdictUsed
}

// coveredByProxy names the servers (by the agent's key, as the hook sees them)
// whose calls mcp-proxy already counted: every tool the hook saw them call is a
// tool one proxied server lists. A direct server that shares a common tool name
// (search, query) with a proxied one is not covered unless all of its tools are.
func coveredByProxy(cmds []ledger.CommandDay, listedBy map[string]map[string]bool) map[string]bool {
	seen := map[string]map[string]bool{}
	for _, c := range cmds {
		if server, name, ok := splitMCPName(c.Command); ok {
			if seen[server] == nil {
				seen[server] = map[string]bool{}
			}
			seen[server][name] = true
		}
	}
	covered := map[string]bool{}
	for server, tools := range seen {
		for _, listed := range listedBy {
			if allIn(tools, listed) {
				covered[server] = true
				break
			}
		}
	}
	return covered
}

func allIn(sub, set map[string]bool) bool {
	for k := range sub {
		if !set[k] {
			return false
		}
	}
	return true
}

// lazyModeTool is one of the two tools mcp-proxy adds to reach lazy tools.
func lazyModeTool(name string) bool { return name == "search_tools" || name == "call_tool" }

// splitMCPName splits mcp__server__tool (Claude and Codex naming).
func splitMCPName(name string) (server, tool string, ok bool) {
	rest, ok := strings.CutPrefix(name, "mcp__")
	if !ok {
		return "", "", false
	}
	server, tool, ok = strings.Cut(rest, "__")
	if !ok || server == "" || tool == "" || server == "gascoigne" {
		return "", "", false
	}
	if lazyModeTool(tool) { // mcp-proxy's lazy tools: the real call is counted there
		return "", "", false
	}
	return server, tool, true
}

// reviewNotes says what the numbers cannot show.
func reviewNotes(r mcpReview) []string {
	notes := []string{}
	if len(r.Servers) == 0 {
		return []string{"No MCP tool data yet. Servers started through gascoigne mcp-proxy report their tools and calls; other servers show up here once the agent calls them."}
	}
	if r.Days < observeDays {
		notes = append(notes, fmt.Sprintf("Only %d day(s) of data: no server is called unused before %d days of observation.", r.Days, observeDays))
	}
	for _, s := range r.Servers {
		if !s.Sized {
			notes = append(notes, "Servers without a size stay direct (they do not go through mcp-proxy): only their calls are known, and a direct server that was never called does not appear. gascoigne install -g routes stdio servers through the proxy.")
			break
		}
	}
	return notes
}

// printReview writes the review as a table, the terminal form of the page.
func printReview(w io.Writer, r mcpReview) {
	if len(r.Servers) == 0 {
		fmt.Fprintln(w, r.Notes[0])
		return
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "SERVER\tVERDICT\tTOKENS/REQ\tIDLE TOKENS\tCALLS 7D\tPREV 7D\tLAST USED")
	for _, s := range r.Servers {
		size, idle := "?", "?"
		if s.Sized {
			size, idle = fmt.Sprint(s.Tokens), fmt.Sprint(s.IdleToks)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\n", s.Name, s.Verdict, size, idle, s.Calls7, s.CallsPrev, cmp.Or(s.LastUsed, "-"))
	}
	tw.Flush()
	for _, n := range r.Notes {
		fmt.Fprintln(w, "\n"+n)
	}
}

// loadReview reads the last two weeks from the ledger.
func loadReview(ctx context.Context, l *ledger.Ledger, now time.Time) (mcpReview, error) {
	since := now.AddDate(0, 0, -2*7)
	stats, err := l.ToolStats(ctx, since)
	if err != nil {
		return mcpReview{}, err
	}
	cmds, err := l.MCPCommands(ctx, since)
	if err != nil {
		return mcpReview{}, err
	}
	return buildReview(stats, cmds, now), nil
}

// cmdMCPReview opens the review page; the table is the fallback for a
// terminal without a browser.
func cmdMCPReview(ctx context.Context, args []string) error {
	var asJSON, text bool
	open := true
	return withLedger(ctx, "mcp-review", args, 0, func(fs *flag.FlagSet) {
		fs.BoolVar(&asJSON, "json", false, "print the review as JSON")
		fs.BoolVar(&text, "text", false, "print the table instead of opening the page")
		fs.BoolVar(&open, "open", true, "open the page in a browser; false only prints its address")
	}, func(l *ledger.Ledger, _ *common, _ []string) error {
		if !asJSON && !text && canBrowse() {
			link := "http://" + serverAddr() + "/mcp"
			fmt.Println(link)
			if open {
				openBrowser(link)
			}
			return nil
		}
		review, err := loadReview(ctx, l, time.Now())
		if err != nil {
			return err
		}
		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(review)
		}
		printReview(os.Stdout, review)
		return nil
	})
}

// canBrowse reports whether the review page can be shown: a display, no CI,
// and the background server answering.
func canBrowse() bool {
	return !serverOff() && os.Getenv("CI") == "" && hasDisplay(runtime.GOOS) && serverUp() == nil
}

// labelDuplicates adds the command hash to servers that share a name.
func labelDuplicates(servers []mcpServer) {
	count := map[string]int{}
	for _, s := range servers {
		count[s.Name]++
	}
	for i, s := range servers {
		if _, hash, ok := strings.Cut(s.ID, "#"); ok && count[s.Name] > 1 {
			servers[i].Label = s.Name + " (" + hash + ")"
		}
	}
}
